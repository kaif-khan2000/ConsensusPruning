package DB_Handler

import (
	"database/sql"
	"encoding/hex"
	"fmt"

	_ "github.com/go-sql-driver/mysql"
)

//this is defined in the Data_Handler package but I don't want to import it
//as it will cause a circular dependency. Therefore defining it here again

type Transaction struct {
	//Type 0: MHT Tx
	//Type 1: IH Tx
	Type int8
	// POW Hash
	Hash [32]byte
	// Address
	From      [65]byte
	LeftTip   [32]byte
	RightTip  [32]byte
	Nonce     uint32
	Timestamp int64
	SessionId [32]byte

	//type 1 tx only
	MerkleRoot [32]byte
	//type 2 tx only
	IntermediateHashes [][32]byte
}

func LoginToMysql(username string, password string) (*sql.DB, error) {
	db, err := sql.Open("mysql", username+":"+password+"@tcp(localhost:3306)/storage_node")
	if err != nil {
		fmt.Println("Error connecting to database: ", err.Error())
		return nil, err
	}
	return db, nil
}

func CreateBlockchainTables(db *sql.DB) error {

	// Create the transactions table
	query := "CREATE TABLE transactions (" +
		"type INT," +
		"hash VARCHAR(64), " +
		"from_address VARCHAR(255), " +
		"left_tip VARCHAR(64) , " +
		"right_tip VARCHAR(64), " +
		"nonce BIGINT, timestamp BIGINT, " +
		"session_id VARCHAR(64), " +
		"merkle_root VARCHAR(64), " +
		// also involve the IH
		"signature VARCHAR(255), " +
		"FOREIGN KEY (left_tip) REFERENCES transactions(hash), " +
		"FOREIGN KEY (right_tip) REFERENCES transactions(hash), " +
		"PRIMARY KEY (hash)); "
	_, err := db.Exec(query)
	if err != nil {
		fmt.Println(query)
		fmt.Println("Error creating blockchain table: ", err.Error())
		return err
	}

	// create IH table
	query = "CREATE TABLE intermediate_hashes (" +
		"hash VARCHAR(64), " +
		"tx_hash VARCHAR(64), " +
		"sequence INT, " +
		"FOREIGN KEY (tx_hash) REFERENCES transactions(hash), " +
		"PRIMARY KEY (hash)); "
	_, err = db.Exec(query)

	if err != nil {
		fmt.Println(query)
		fmt.Println("Error creating intermediate hashes table: ", err.Error())
		return err
	}

	return nil
}

func InsertTransaction(db *sql.DB, tx Transaction, Signature []byte) error {
	// Insert the transaction
	query := "INSERT INTO transactions (type, hash, from_address, left_tip, right_tip, nonce, timestamp, session_id, merkle_root, signature) VALUES ("
	query += fmt.Sprintf("%d, ", tx.Type)
	query += fmt.Sprintf("'%s', ", hex.EncodeToString(tx.Hash[:]))
	query += fmt.Sprintf("'%s', ", hex.EncodeToString(tx.From[:]))
	query += fmt.Sprintf("'%s', ", hex.EncodeToString(tx.LeftTip[:]))
	query += fmt.Sprintf("'%s', ", hex.EncodeToString(tx.RightTip[:]))
	query += fmt.Sprintf("%d, ", tx.Nonce)
	query += fmt.Sprintf("%d, ", tx.Timestamp)
	query += fmt.Sprintf("'%s', ", hex.EncodeToString(tx.SessionId[:]))
	query += fmt.Sprintf("'%s', ", hex.EncodeToString(tx.MerkleRoot[:]))
	query += fmt.Sprintf("'%s'); ", hex.EncodeToString(Signature))
	_, err := db.Exec(query)
	if err != nil {
		fmt.Println(query)
		fmt.Println("Error inserting transaction: ", err.Error())
		return err
	}
	query = ""
	// insert intermediate hashes into the table
	for i, hash := range tx.IntermediateHashes {
		query = "INSERT INTO intermediate_hashes (hash, tx_hash, sequence) VALUES ("
		query += fmt.Sprintf("'%s', ", hex.EncodeToString(hash[:]))
		query += fmt.Sprintf("'%s', ", hex.EncodeToString(tx.Hash[:]))
		query += fmt.Sprintf("%d); ", i)
		if query != "" {
			_, err = db.Exec(query)
			if err != nil {
				fmt.Println(query)
				fmt.Println("Error inserting intermediate hashes: ", err.Error())
				return err
			}
		}
	}

	return nil
}

func InsertGenesisTx(db *sql.DB, tx Transaction) error {
	query := "INSERT INTO transactions (type, hash, from_address, nonce, timestamp) VALUES ("
	query += fmt.Sprintf("%d, ", tx.Type)
	query += fmt.Sprintf("'%s', ", hex.EncodeToString(tx.Hash[:]))
	query += fmt.Sprintf("'%s', ", hex.EncodeToString(tx.From[:]))
	query += fmt.Sprintf("%d, ", tx.Nonce)
	query += fmt.Sprintf("%d ", tx.Timestamp)
	query += "); "
	_, err := db.Exec(query)
	if err != nil {
		fmt.Println(query)
		fmt.Println("Error inserting genesis transaction: ", err.Error())
		return err
	}

	return nil
}
