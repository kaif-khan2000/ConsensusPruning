package main

import (
	"bufio"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

type Record struct {
	// the type of the data
	Type      string
	Timestamp int64
	Data      float32
	CrateId   string
	GwID      string
}

type CssData struct {
	SessionID     string
	IhSequence    int
	ChunkPosition int
	RecordArray   []Record
	Ihtx_id       string
	IhStart       int
	IhEnd         int
	Mhtx_id       string
}

type CssMessage struct {
	// the type of the message
	Type int // 0 = chunk, 1 = ihTx, 2 = mhTx
	// the data of the message
	Data CssData
}

type SessionInfo struct {
	// session Id
	SessionId [32]byte
	// Total No.of chunks in the session
	TotalChunks int
	// No.of chunks requested
	ChunksRequested int
}

type CssInfo struct {
	// session Ids
	Sessions []SessionInfo
}

const (
	CSS_IP     = "150.0.0.200"
	CSS_PORT   = 9090
	MYSQL_IP   = "150.0.0.254"
	MYSQL_HOST = "mysql"
	MYSQL_PORT = 3306
)

func Serialize(p interface{}) ([]byte, error) {
	return json.Marshal(p)
}

// deserializes the byte array to an object
func Deserialize(data string, p any) error {
	return json.Unmarshal(json.RawMessage(data), p)
}

var db *sql.DB

func LoginToMysql(username string, password string) (*sql.DB, error) {
	// db, err := sql.Open("mysql", username+":"+password+"@tcp("+MYSQL_HOST+":3306)/css")
	db, err := sql.Open("mysql", username+":"+password+"@tcp(localhost:3306)/css")
	if err != nil {
		fmt.Println("Error connecting to database: ", err.Error())
		return nil, err
	}
	return db, nil
}

func CreateCSSTables(db *sql.DB) error {
	// create the chunk table
	query := "CREATE TABLE chunks (" +
		"chunk_id INT NOT NULL AUTO_INCREMENT," +
		"session_id VARCHAR(64) NOT NULL," +
		"ih_sequence INT NOT NULL," +
		"chunk_position INT NOT NULL, PRIMARY KEY (chunk_id) );"
	_, err := db.Exec(query)
	if err != nil {
		fmt.Println("Error creating chunks table: ", err.Error())
		return err
	}

	// create the record table
	query = "CREATE TABLE records (" +
		"record_id INT NOT NULL AUTO_INCREMENT," +
		"type varchar(50) NOT NULL," +
		"timestamp BIGINT NOT NULL," +
		"data FLOAT NOT NULL," +
		"crate_id varchar(50) NOT NULL," +
		"gw_id varchar(50) NOT NULL," +
		"chunk_id INT NOT NULL," +
		"PRIMARY KEY (record_id), FOREIGN KEY (chunk_id) REFERENCES chunks(chunk_id) );"

	_, err = db.Exec(query)
	if err != nil {
		fmt.Println("Error creating records table: ", err.Error())
		return err
	}

	// create the IhTx Info table
	query = "CREATE TABLE ihtx_info (" +
		"session_id VARCHAR(64) NOT NULL," +
		"ih_sequence INT NOT NULL," +
		"ihtx_id varchar(64) NOT NULL);"

	_, err = db.Exec(query)
	if err != nil {
		fmt.Println("Error creating ihtx_info table: ", err.Error())
		return err
	}

	// crate MhTx
	query = "CREATE TABLE mhtx_info (" +
		"session_id VARCHAR(64) NOT NULL, " +
		"mhtx_id VARCHAR(64), PRIMARY KEY (session_id));"
	_, err = db.Exec(query)
	if err != nil {
		fmt.Println("Error creating mhtx_info table: ", err.Error())
		return err
	}

	return nil
}

func homePage(c *gin.Context) {
	c.JSON(200, gin.H{
		"message": "Hello World!",
	})
}

func addChunkToDB(chunk CssData) error {
	// insert the chunk into the database
	query := "INSERT INTO chunks (session_id, ih_sequence, chunk_position) VALUES (?, ?, ?);"
	stmt, err := db.Prepare(query)
	if err != nil {
		fmt.Println("Error preparing statement: ", err.Error())
		return err
	}
	defer stmt.Close()
	fmt.Println("Inserting chunk: ", chunk.SessionID, chunk.IhSequence, chunk.ChunkPosition)
	_, err = stmt.Exec(chunk.SessionID, chunk.IhSequence, chunk.ChunkPosition)
	if err != nil {
		fmt.Println("Error executing statement: ", err.Error())
		return err
	}

	// add the records to the database
	query = "INSERT INTO records (type, timestamp, data, crate_id, gw_id, chunk_id) VALUES (?, ?, ?, ?, ?, ?);"
	stmt, err = db.Prepare(query)
	if err != nil {
		fmt.Println("Error preparing statement: ", err.Error())
		return err
	}
	defer stmt.Close()

	//get the chunk id
	var chunkID int
	query = "SELECT chunk_id FROM chunks WHERE session_id = ? AND ih_sequence = ? AND chunk_position = ?;"
	err = db.QueryRow(query, chunk.SessionID, chunk.IhSequence, chunk.ChunkPosition).Scan(&chunkID)
	if err != nil {
		fmt.Println("Error getting chunk id: ", err.Error())
		return err
	}

	// add records one by one
	for _, record := range chunk.RecordArray {
		_, err := stmt.Exec(record.Type, record.Timestamp, record.Data, record.CrateId, record.GwID, chunkID)
		if err != nil {
			fmt.Println("Error executing statement: ", err.Error())
			return err
		}
	}
	return nil
}

func addIhTxInfoToDB(ihTxInfo CssData) error {
	// insert the ihTxInfo into the database
	query := "INSERT INTO ihtx_info (session_id, ih_sequence, ihtx_id) VALUES (?, ?, ?);"
	stmt, err := db.Prepare(query)
	if err != nil {
		fmt.Println("Error preparing statement: ", err.Error())
		return err
	}
	defer stmt.Close()

	for seq := ihTxInfo.IhStart; seq <= ihTxInfo.IhEnd; seq++ {
		_, err = stmt.Exec(ihTxInfo.SessionID, seq, ihTxInfo.Ihtx_id)
		if err != nil {
			fmt.Println("Error executing statement: ", err.Error())
			return err
		}
	}
	return nil
}

func addMhTxInfoToDB(mhTxInfo CssData) error {
	// insert the mhTxInfo into the database
	query := "INSERT INTO mhtx_info (session_id, mhtx_id) VALUES (?, ?);"
	stmt, err := db.Prepare(query)
	if err != nil {
		fmt.Println("Error preparing statement: ", err.Error())
		return err
	}
	defer stmt.Close()

	_, err = stmt.Exec(mhTxInfo.SessionID, mhTxInfo.Mhtx_id)
	if err != nil {
		fmt.Println("Error executing statement: ", err.Error())
		return err
	}
	return nil
}

func addChunk(c *gin.Context) {
	var chunk CssData
	c.BindJSON(&chunk)
	err := addChunkToDB(chunk)
	if err != nil {
		c.JSON(500, gin.H{
			"message": "Error adding chunk to database",
		})
		return
	}
}

func addIhTxInfo(c *gin.Context) {
	var ihTxInfo CssData
	c.BindJSON(&ihTxInfo)
	err := addIhTxInfoToDB(ihTxInfo)
	if err != nil {
		c.JSON(500, gin.H{
			"message": "Error adding ihTxInfo to database",
		})
		return
	}
	// respond to the client
	c.IndentedJSON(200, ihTxInfo)
}

func addMhTxInfo(c *gin.Context) {
	var mhTxInfo CssData
	c.BindJSON(&mhTxInfo)
	err := addMhTxInfoToDB(mhTxInfo)
	if err != nil {
		c.JSON(500, gin.H{
			"message": "Error adding mhTxInfo to database",
		})
		return
	}
	// respond to the client
	c.IndentedJSON(200, mhTxInfo)
}

func recvChunk(c *gin.Context) {
	a := c.Param("SessionId")
	var b string = c.Param("IhSequence")
	var d string = c.Param("ChunkPosition")
	var chunk CssData
	chunk.SessionID = a
	b1, err := strconv.Atoi(b)
	if err != nil {
		fmt.Println("Error converting ih sequence to int: ", err.Error())
		return
	}
	chunk.IhSequence = b1
	// println(reflect.TypeOf(d))
	d1, err := strconv.Atoi(d)
	if err != nil {
		fmt.Println("Error converting chunk position to int: ", err.Error())
		return
	}
	chunk.ChunkPosition = d1
	// fmt.Println("chunk: ", chunk.chunk_Id)

	// get the chunk id
	var chunkID int
	query := "SELECT chunk_id FROM chunks WHERE session_id = ? AND ih_sequence = ? AND chunk_position = ?;"
	err = db.QueryRow(query, a, b, d).Scan(&chunkID)
	if err != nil {
		fmt.Println("Error getting chunk id: ", err.Error())
		c.JSON(500, gin.H{
			"message": "Error getting chunk id",
		})
		return
	}
	fmt.Println("chunk: ", chunkID)
	// get the records from the database
	query = "SELECT type, timestamp, data, crate_id, gw_id FROM records WHERE chunk_id = ?;"
	rows, err := db.Query(query, chunkID)
	if err != nil {
		fmt.Println("Error getting records: ", err.Error())
		c.JSON(500, gin.H{
			"message": "Error getting records",
		})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var temp Record
		err := rows.Scan(&temp.Type, &temp.Timestamp, &temp.Data, &temp.CrateId, &temp.GwID)
		if err != nil {
			fmt.Print("Unable to fetch whole data", err)
			return
		}
		// fmt.Println(temp.Type)
		chunk.RecordArray = append(chunk.RecordArray, temp)
	}
	println("chunk RecordArray: ", len(chunk.RecordArray))
	c.IndentedJSON(200, chunk)
}

func recvIhTxInfo(c *gin.Context) {
	a := c.Param("SessionId")
	var b string = c.Param("ihtx_id")
	var ihTxInfo CssData
	ihTxInfo.SessionID = a
	ihTxInfo.Ihtx_id = b

	// c.BindJSON(&ihTxInfo)
	fmt.Println("ihTxInfo: ", ihTxInfo)

	// get the ihTxInfo from the database
	query := "SELECT ih_sequence, ihtx_id FROM ihtx_info WHERE session_id = ? AND ihtx_id = ?;"
	rows, err := db.Query(query, a, b)
	if err != nil {
		fmt.Println("Error getting ihTxInfo: ", err.Error())
		c.JSON(500, gin.H{
			"message": "Error getting ihTxInfo",
		})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var temp CssData
		err := rows.Scan(&temp.IhSequence, &temp.Ihtx_id)
		if err != nil {
			fmt.Print("Unable to fetch whole data", err)
			return
		}
		ihTxInfo.IhSequence = temp.IhSequence
		ihTxInfo.Ihtx_id = temp.Ihtx_id
	}

	// respond to the client
	c.IndentedJSON(200, ihTxInfo)
}

func recvMhTxInfo(c *gin.Context) {
	a := c.Param("MHTxinfo")
	var mhTxInfo CssData
	Deserialize(a, &mhTxInfo)
	// c.BindJSON(&mhTxInfo)
	fmt.Println("mhTxInfo: ", mhTxInfo)

	// get the mhTxInfo from the database
	query := "SELECT mhtx_id FROM mhtx_info WHERE session_id = ?;"
	rows, err := db.Query(query, mhTxInfo.SessionID)
	if err != nil {
		fmt.Println("Error getting mhTxInfo: ", err.Error())
		c.JSON(500, gin.H{
			"message": "Error getting mhTxInfo",
		})
		return
	}
	defer rows.Close()

	for rows.Next() {
		var temp CssData
		err := rows.Scan(&temp.Mhtx_id)
		if err != nil {
			fmt.Print("Unable to fetch whole data", err)
			return
		}
		mhTxInfo.Mhtx_id = temp.Mhtx_id
	}

	// respond to the client
	c.IndentedJSON(200, mhTxInfo)
}

func recvChunks(c *gin.Context) {
	gw_id := c.Param("GwID")
	start := c.Param("start")
	end := c.Param("end")
	chunks := make([]CssData, 0)
	fmt.Println("gw_id: ", gw_id)
	fmt.Println("start: ", start)
	fmt.Println("end: ", end)
	// get the chunk id of the records where time is between start and end and gw_id is gw_id
	query := "SELECT chunk_id FROM records WHERE gw_id = ? AND timestamp BETWEEN ? AND ?;"
	rows, err := db.Query(query, gw_id, start, end)
	if err != nil {
		fmt.Println("Error getting chunk id: ", err.Error())
		return
	}
	// get all the chunks based on the chunk id in rows

	for rows.Next() {
		var temp CssData
		var chunk_id int
		err := rows.Scan(&chunk_id)
		if err != nil {
			fmt.Print("Unable to fetch whole data", err)
			return
		}
		fmt.Println("chunk_id: ", chunk_id)

		// get the chunk info from the database
		query = "SELECT session_id, ih_sequence, chunk_position FROM chunks WHERE chunk_id = ?;"
		rows, err := db.Query(query, chunk_id)
		if err != nil {
			fmt.Println("Error getting chunk info: ", err.Error())
			return
		}

		for rows.Next() {
			err := rows.Scan(&temp.SessionID, &temp.IhSequence, &temp.ChunkPosition)
			if err != nil {
				fmt.Print("Unable to fetch whole data", err)
				return
			}
		}

		// get all records in the chunk
		query = "SELECT type, timestamp, data, crate_id, gw_id FROM records WHERE chunk_id = ?;"
		rows, err = db.Query(query, chunk_id)
		if err != nil {
			fmt.Println("Error getting chunk info: ", err.Error())
			return
		}
		for rows.Next() {
			var temp2 Record
			err := rows.Scan(&temp2.Type, &temp2.Timestamp, &temp2.Data, &temp2.CrateId, &temp2.GwID)
			if err != nil {
				fmt.Print("Unable to fetch whole data", err)
				return
			}
			temp.RecordArray = append(temp.RecordArray, temp2)
		}

		chunks = append(chunks, temp)
	}
	c.IndentedJSON(200, chunks)
}

func getChunks(c *gin.Context) {
	session_id := c.Param("SessionId")
	start := c.Param("start")
	end := c.Param("end")
	chunks := make([]CssData, 0)
	fmt.Println("session_id: ", session_id)
	fmt.Println("start: ", start)
	fmt.Println("end: ", end)
	// get the chunk id of the records where time is between start and end and gw_id is gw_id
	query := "SELECT DISTINCT c.chunk_id FROM chunks as c,records as r WHERE c.chunk_id = r.chunk_id AND session_id = ? AND timestamp BETWEEN ? AND ?"
	rows, err := db.Query(query, session_id, start, end)
	if err != nil {
		fmt.Println("Error getting chunk id: ", err.Error())
		return
	}
	// get all the chunks based on the chunk id in rows
	for rows.Next() {
		var temp CssData
		var chunk_id int
		err := rows.Scan(&chunk_id)
		if err != nil {
			fmt.Print("Unable to fetch whole data", err)
			return
		}
		fmt.Println("chunk_id: ", chunk_id)

		// get the chunk info from the database
		query = "SELECT session_id, ih_sequence, chunk_position FROM chunks WHERE chunk_id = ?;"
		rows, err := db.Query(query, chunk_id)
		if err != nil {
			fmt.Println("Error getting chunk info: ", err.Error())
			return
		}

		for rows.Next() {
			err := rows.Scan(&temp.SessionID, &temp.IhSequence, &temp.ChunkPosition)
			if err != nil {
				fmt.Print("Unable to fetch whole data", err)
				return
			}
		}

		// get ihtx info
		query = "SELECT ihtx_id FROM ihtx_info WHERE session_id = ? AND ih_sequence = ?;"
		rows, err = db.Query(query, temp.SessionID, temp.IhSequence)
		if err != nil {
			fmt.Println("Error getting ihtx info: ", err.Error())
			return
		}
		for rows.Next() {
			err := rows.Scan(&temp.Ihtx_id)
			if err != nil {
				fmt.Print("Unable to fetch whole data", err)
				return
			}
		}

		// get all records in the chunk
		query = "SELECT type, timestamp, data, crate_id, gw_id FROM records WHERE chunk_id = ? order by timestamp asc;"
		rows, err = db.Query(query, chunk_id)
		if err != nil {
			fmt.Println("Error getting chunk info: ", err.Error())
			return
		}
		for rows.Next() {
			var temp2 Record
			err := rows.Scan(&temp2.Type, &temp2.Timestamp, &temp2.Data, &temp2.CrateId, &temp2.GwID)
			if err != nil {
				fmt.Print("Unable to fetch whole data", err)
				return
			}
			temp.RecordArray = append(temp.RecordArray, temp2)
		}

		chunks = append(chunks, temp)
	}
	c.IndentedJSON(200, chunks)
}

func getInfo(c *gin.Context) {
	gw_id := c.Param("GwID")
	start := c.Param("start")
	end := c.Param("end")

	var info CssInfo

	// select the session to which the chunks belong to where it contains the gw_id and the time is between start and end
	query := "SELECT DISTINCT session_id FROM chunks as c, records as r WHERE c.chunk_id = r.chunk_id AND gw_id = ? AND timestamp BETWEEN ? AND ?;"
	rows, err := db.Query(query, gw_id, start, end)
	if err != nil {
		fmt.Println("Error getting session id: ", err.Error())
		return
	}

	for rows.Next() {
		var temp SessionInfo
		var session_id string
		err := rows.Scan(&session_id)
		hex.Decode(temp.SessionId[:], []byte(session_id))
		if err != nil {
			fmt.Print("Unable to fetch whole data", err)
			return
		}
		// add tho info
		info.Sessions = append(info.Sessions, temp)
	}

	// get the total number of chunks in a session and the total chunks actually lies in the time range
	for i := 0; i < len(info.Sessions); i++ {
		query = "SELECT COUNT(chunk_id) FROM chunks WHERE session_id = ?;"
		temp_session := hex.EncodeToString(info.Sessions[i].SessionId[:])
		rows, err := db.Query(query, temp_session)
		if err != nil {
			fmt.Println("Error getting chunk id: -  ", err.Error())
			return
		}

		for rows.Next() {
			err := rows.Scan(&info.Sessions[i].TotalChunks)
			if err != nil {
				fmt.Print("Unable to fetch whole data1: ", err)
				return
			}
		}

		query = "SELECT COUNT(distinct c.chunk_id) FROM chunks as c, records as r WHERE c.chunk_id = r.chunk_id AND session_id = ? AND timestamp BETWEEN ? AND ?;"
		rows, err = db.Query(query, temp_session, start, end)
		if err != nil {
			fmt.Println("Error getting chunk id: 2-", err.Error())
			return
		}

		for rows.Next() {
			err := rows.Scan(&info.Sessions[i].ChunksRequested)
			if err != nil {
				fmt.Print("Unable to fetch whole data2: ", err)
				return
			}
		}
	}

	c.IndentedJSON(200, info)
}

func getAllChunks(c *gin.Context) {
	session_id := c.Param("SessionId")
	fmt.Println("session_id: ", session_id)

	// get MHTxInfo
	query := "SELECT mhtx_id FROM mhtx_info WHERE session_id = ?;"
	rows, err := db.Query(query, session_id)
	if err != nil {
		fmt.Println("Error getting mhtx info: ", err.Error())
		return
	}
	var mhtx_id string
	for rows.Next() {
		err := rows.Scan(&mhtx_id)
		if err != nil {
			fmt.Print("Unable to fetch whole data", err)
			return
		}
	}
	fmt.Println("mhtx_id: ", mhtx_id)

	query = "SELECT chunk_id, session_id, ih_sequence, chunk_position FROM chunks WHERE session_id = ? ORDER BY ih_sequence ASC, chunk_position ASC;"
	rows, err = db.Query(query, session_id)

	if err != nil {
		fmt.Println("Error getting chunk id: ", err.Error())
		return
	}

	var chunks []CssData
	for rows.Next() {
		var temp CssData
		var chunk_id int
		err = rows.Scan(&chunk_id, &temp.SessionID, &temp.IhSequence, &temp.ChunkPosition)
		if err != nil {
			fmt.Print("Unable to fetch whole data", err)
			return
		}
		temp.Mhtx_id = mhtx_id

		// get all records in the chunk
		query = "SELECT type, timestamp, data, crate_id, gw_id FROM records WHERE chunk_id = ?;"
		rows2, err2 := db.Query(query, chunk_id)
		if err2 != nil {
			fmt.Println("Error getting chunk info: ", err.Error())
			return
		}
		for rows2.Next() {
			var temp2 Record
			err := rows2.Scan(&temp2.Type, &temp2.Timestamp, &temp2.Data, &temp2.CrateId, &temp2.GwID)
			if err != nil {
				fmt.Print("Unable to fetch whole data", err)
				return
			}
			temp.RecordArray = append(temp.RecordArray, temp2)
		}
		chunks = append(chunks, temp)
	}
	c.IndentedJSON(200, chunks)
}

func dbServer() {
	route := gin.Default()
	route.GET("/", homePage)
	route.POST("/addChunk", addChunk)
	route.POST("/addIhTxInfo", addIhTxInfo)
	route.POST("/addMhTxInfo", addMhTxInfo)
	route.GET("/recvChunk/:SessionId/:IhSequence/:ChunkPosition", recvChunk)
	route.GET("/recvIh/:SessionId/:ihtx_id", recvIhTxInfo)
	route.GET("/recvMhTxInfo/:MHTxinfo", recvMhTxInfo)
	route.GET("/recvChunks/:GwID/:start/:end", recvChunks)
	route.GET("/getChunks/:SessionId/:start/:end", getChunks)
	route.GET("/getAllChunks/:SessionId", getAllChunks)
	route.GET("/getInfo/:GwID/:start/:end", getInfo)
	route.Run("0.0.0.0:9090")
}

func ClearDatabase() {
	// remove records table
	query := "DROP TABLE IF EXISTS records;"
	_, err := db.Exec(query)
	if err != nil {
		fmt.Println("Error dropping records table: ", err.Error())
	}

	// remove chunks table
	query = "DROP TABLE IF EXISTS chunks;"
	_, err = db.Exec(query)
	if err != nil {
		fmt.Println("Error dropping chunks table: ", err.Error())
	}

	// remove ihtx_info table
	query = "DROP TABLE IF EXISTS ihtx_info;"
	_, err = db.Exec(query)
	if err != nil {
		fmt.Println("Error dropping ihtx_info table: ", err.Error())
	}

	// remove mhtx_info table
	query = "DROP TABLE IF EXISTS mhtx_info;"
	_, err = db.Exec(query)
	if err != nil {
		fmt.Println("Error dropping mhtx_info table: ", err.Error())
	}

	// now create the tables
	err = CreateCSSTables(db)
	if err != nil {
		fmt.Println("Error creating tables: ", err.Error())
		return
	}
}

func processMessage(message CssMessage) {
	// now process the message
	var err error
	if message.Type == 0 {
		chunk := message.Data
		err = addChunkToDB(chunk)
		if err != nil {
			fmt.Println("Error adding chunk to database: ", err.Error())
			return
		}
		fmt.Println("Added chunk to database: ", chunk.IhSequence, chunk.ChunkPosition)
	} else if message.Type == 1 {
		ihTxInfo := message.Data
		err = addIhTxInfoToDB(ihTxInfo)
		if err != nil {
			fmt.Println("Error adding ihTxInfo to database: ", err.Error())
			return
		}
		fmt.Println("Added ihTxInfo to database")
	} else if message.Type == 2 {
		mhTxInfo := message.Data
		err = addMhTxInfoToDB(mhTxInfo)
		if err != nil {
			fmt.Println("Error adding mhTxInfo to database: ", err.Error())
			return
		}
		fmt.Println("Added mhTxInfo to database")
	}
}

func handleRequest(conn net.Conn) {
	for {
		// read the message
		msg, err := bufio.NewReader(conn).ReadString('\n')
		if err != nil {
			fmt.Println("Error reading: ", err.Error())
			return
		}
		msg = strings.Trim(msg, "\n")
		byteMsg := []byte(msg)
		var message CssMessage
		err = json.Unmarshal(byteMsg, &message)
		if err != nil {
			fmt.Println("Error unmarshalling: ", err.Error())
			return
		}
		go processMessage(message)
	}
}

func createServer() {
	ln, err := net.Listen("tcp", ":9091")
	if err != nil {
		fmt.Println("Error listening: ", err.Error())
		return
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting: ", err.Error())
			return
		}
		go handleRequest(conn)
	}
}

func main() {
	var err error
	username := "kaif"
	password := "kaif"
	// time.Sleep(30 * time.Second)
	db, err = LoginToMysql(username, password)
	if err != nil {
		fmt.Println("Error connecting to database: ", err.Error())
		return
	} else {
		fmt.Println("Connected to database")
	}

	// time.Sleep(200 * time.Second)
	//wait till the database is up and running
	//running dummy query to check if the database is up and running
	for {
		query := "SHOW TABLES;"
		_, err = db.Exec(query)
		if err != nil {
			fmt.Println("wait for db", err.Error())
		} else {
			break
		}
	}
	// remove this later
	fmt.Println("Clearing database")
	ClearDatabase()

	// err = CreateCSSTables(db)
	// if err != nil {
	// 	fmt.Println("Error creating tables: ", err.Error())
	// 	return
	// }

	//TCP Server
	go createServer()

	// Http Server
	dbServer()
	defer db.Close()
}
