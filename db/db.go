package db

import (
	"bytes"
	"compress/zlib"
	"database/sql"
	"errors"
	"fmt"
	"github.com/mattn/go-sqlite3"
)

const schemaVersion = 1
const dbSchema string = `
CREATE TABLE IF NOT EXISTS messages (sender TEXT NOT NULL, send_time TEXT NOT NULL, channel TEXT NOT NULL, message BLOB, PRIMARY KEY(send_time, channel));
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
	dbConn  *sql.DB
	initErr error
)

type SearchResult struct {
	Msg Message
}

type Message struct {
	Sender   string
	Channel  string
	SendTime string
	Message  string
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
func AddMessages(messages []Message) (err error) {
	if initErr != nil {
		return initErr
	}

	tx, err := dbConn.Begin()

	if err != nil {
		return err
	}

	for _, msg := range messages {
		stmt, err := dbConn.Prepare("INSERT INTO messages (sender, channel, send_time, message) VALUES (?, ?, ?, ?);")
		if err != nil {
			tx.Rollback()
			return err
		}
		_, err = tx.Stmt(stmt).Exec(msg.Sender, msg.Channel, msg.SendTime, msg.Message)
		if err != nil {
			tx.Rollback()
			return err
		}
	}

	tx.Commit()

	return nil
}

// Search for messages matching the given parameters
func SearchMessage(sender string, channel string, query string) (results []SearchResult, err error) {
	if initErr != nil {
		return results, initErr
	}

	stmtFragment := "SELECT sender, channel, send_time, decompress(message) FROM messages_idx WHERE channel=\"%s\" AND messages_idx MATCH '%s'"
	var stmtStr string
	if sender != "" {
		stmtStr = fmt.Sprintf(stmtFragment+" AND sender=\"%s\";", channel, query, sender)
	} else {
		stmtStr = fmt.Sprintf(stmtFragment+";", channel, query)
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
			var sendTimeRes string
			var messageRes string

			if err := rows.Scan(&senderRes, &channelRes, &sendTimeRes, &messageRes); err != nil {
				return results, err
			} else {
				results = append(results, SearchResult{Msg: Message{Sender: senderRes, Channel: channelRes, SendTime: sendTimeRes, Message: messageRes}})
			}
		}
	}

	return results, nil
}

func GetLatestMessageTime(channel string) (string, error) {
	stmtStr := fmt.Sprintf("SELECT MAX(send_time) FROM messages WHERE channel=\"%s\";", channel)

	stmt, err := dbConn.Prepare(stmtStr)
	if err != nil {
		return "", err
	}

	rows, err := stmt.Query()
	if err != nil {
		return "", err
	}

	defer rows.Close()

	var maxTime string = ""
	for rows.Next() {
		rows.Scan(&maxTime)
	}

	return maxTime, nil
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
func compressMessage(message string) []byte {
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
		if _, err = dbConn.Exec("INSERT INTO versions VALUES (?)", schemaVersion); err != nil {
			return -1
		}
		return schemaVersion
	}

	return maxVersion
}
