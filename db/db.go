package db

import (
	"fmt"
	"bytes"
	"database/sql"
	"compress/zlib"
	"github.com/mattn/go-sqlite3"
	"errors"
)

const schemaVersion = 1
const dbSchema string = `
CREATE TABLE IF NOT EXISTS messages (sender TEXT NOT NULL, send_time INTEGER NOT NULL, channel TEXT NOT NULL, message BLOB);
CREATE VIRTUAL TABLE IF NOT EXISTS messages_idx USING fts5 (content=messages, content_rowid=rowid, sender UNINDEXED, send_time UNINDEXED, channel UNINDEXED, message);

-- Triggers to keep the FTS index up to date.
CREATE TRIGGER IF NOT EXISTS messages_insert AFTER INSERT ON messages BEGIN
	INSERT INTO messages_idx(rowid, message) VALUES (new.rowid, new.message);
	UPDATE messages SET message=compress(new.message) WHERE rowid=new.rowid;
END;
CREATE TRIGGER IF NOT EXISTS messages_delete AFTER DELETE ON messages BEGIN
	INSERT INTO messages_idx(fts_idx, rowid, message) VALUES('delete', old.rowid, decompress(old.message));
END;
`

var (
	dbConn *sql.DB
	initErr error
)

type SearchResult struct {
	sender string
	channel string
	sendTime int
	message string
}

func init() {
	// Register custom db functions to compress/decompress messages
	sql.Register("sqlite3_custom", &sqlite3.SQLiteDriver{
		ConnectHook: func(conn *sqlite3.SQLiteConn) error {
			if initErr = conn.RegisterFunc("compress", compressMessage, true); initErr != nil {
				return initErr
			}
			if initErr := conn.RegisterFunc("decompress", decompressMessage, true); initErr != nil {
				return initErr
			}
			return nil
		},
	})

	dbConn, initErr = sql.Open("sqlite3_custom", "./flashback.db")

	// Verify database schema version matches expected version
	if version := getSchemaVersion(); version != schemaVersion {
		initErr = errors.New(fmt.Sprintf("Mismatching database schema version %d; expect %d", version, schemaVersion))
		return
	}

	if initErr != nil {
		fmt.Println("Error opening database")
		return
	}

	_, initErr = dbConn.Exec(dbSchema)

	if initErr != nil {
		fmt.Printf("Error creating messages table: %q\n", initErr)
		return
	}
}

// Add a message to database
func AddMessage(sender string, channel string, sendTime int, message string) (err error) {
	if initErr != nil {
		return initErr
	}

	stmtStr := fmt.Sprintf("INSERT INTO messages (sender, channel, send_time, message) VALUES (\"%s\", \"%s\", %d, \"%s\");", sender, channel, sendTime, message)
	fmt.Println("AddMessage: " + stmtStr)

	if stmt, err := dbConn.Prepare(stmtStr); err != nil {
		return err
	} else {
		_, err = stmt.Exec()
	}

	return err
}

// Search for messages matching the given parameters
func SearchMessage(sender string, channel string, query string) (results []SearchResult, err error) {
	if initErr != nil {
		return results, initErr
	}

	stmtFragment := "SELECT sender, channel, send_time, decompress(message) FROM messages_idx WHERE channel=\"%s\" AND messages_idx MATCH '%s'"
	var stmtStr string
	if sender != "" {
		stmtStr = fmt.Sprintf(stmtFragment + " AND sender=\"%s\";", channel, query, sender)
	} else {
		stmtStr = fmt.Sprintf(stmtFragment + ";", channel, query)
	}

	fmt.Println("SearchMessage: " + stmtStr)

	stmt, err := dbConn.Prepare(stmtStr)
	if err != nil {
		return results, err
	}

	if rows, err := stmt.Query(); err != nil {
		return results, err
	} else {
		defer rows.Close()

		for rows.Next() {
			var senderRes string
			var channelRes string
			var sendTimeRes int
			var messageRes string

			if err := rows.Scan(&senderRes, &channelRes, &sendTimeRes, &messageRes); err != nil {
				return results, err
			} else {
				results = append(results, SearchResult{sender: senderRes, channel: channelRes, sendTime: sendTimeRes, message: messageRes})
			}
		}
	}

	return results, nil
}

// Close the database connection
func Close() {
	if dbConn != nil {
		dbConn.Close()
	}
}

// Checks if the database is ready
func IsReady() (bool, error) {
	return initErr == nil, initErr
}

// Apply zlib compression to message text
func compressMessage(message string) ([]byte) {
	var buf bytes.Buffer

	if writer, err := zlib.NewWriterLevel(&buf, zlib.BestCompression); err != nil {
		return nil
	} else {
		writer.Write([]byte(message))
		writer.Close()
	}

	return buf.Bytes()
}

// Apply zlib decompression to retrieve original message text
func decompressMessage(message []byte) string {
	reader := bytes.NewReader(message)

	if r, err := zlib.NewReader(reader); err != nil {
		return ""
	} else {
		var buf bytes.Buffer
		buf.ReadFrom(r)
		return buf.String()
	}

	return ""
}

func getSchemaVersion() int {
	if initErr != nil {
		return -1
	}

	stmt := "CREATE TABLE IF NOT EXISTS versions (version integer primary key);"
	if _, err := dbConn.Exec(stmt); err != nil {
		return -1
	}

	stmt = "SELECT COUNT(*) FROM versions;"
	rows, err := dbConn.Query(stmt)
	defer rows.Close()
	if err != nil {
		return -1
	}

	count := 0
	for rows.Next() {
		rows.Scan(&count)
	}

	stmt = "SELECT MAX(version) from versions;"
	rows, err = dbConn.Query(stmt)
	defer rows.Close()
	if err != nil {
		return -1
	}

	maxVersion := 0
	for rows.Next() {
		rows.Scan(&maxVersion)
	}

	if maxVersion < schemaVersion || count == 0 {
		stmt = fmt.Sprintf("INSERT INTO versions VALUES (%d)", schemaVersion)
		if _, err = dbConn.Exec(stmt); err != nil {
			return -1
		}
		return schemaVersion
	}

	return maxVersion
}
