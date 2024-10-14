package P2P_Manager

import (
	dh "LSDI_Gateway/Data_Handler"
	"crypto/ecdsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net"
	"strings"
	"sync"
	"time"
)

const (
	SERVER_PORT = "8080"
	SERVER_HOST = "150.0.0.100"
	// SERVER_HOST = "localhost"
	SERVER_TYPE = "tcp"
	MY_PORT     = "4242"
)

type Message struct {
	Type      string
	Data      string
	Timestamp int64
}

// message format for gateway to gateway communication
type GwMessage struct {
	Data      dh.Vertex
	Token     string
	Timestamp int64
	Msg_type  string
}

// message format for gateway to storage node communication
type send_data_sn struct {
	Own_token     string
	Own_nodeno    int
	Data          string
	Msg_type      string
	Msg_no        int
	Routing_token string
}

// transaction pool
type tx_pool_set struct {
	Tx_pool_set map[string]bool
	tx_pool     []string
	cur_pointer int
	m           sync.Mutex
}

var dag *dh.DAG
var gwPrivateKey *ecdsa.PrivateKey
var neighbours dh.Neighbours_GW     // neighbours of this gateway
var connections map[string]net.Conn // connections to other gateways
var tx_set tx_pool_set              // set of transactions in transaction pool
var my_ipaddress string             // ip address of this gateway
var download_gw_conn net.Conn       // connection to download gateway
// serialize and deserialize
func Serialize(p interface{}) ([]byte, error) {
	return json.Marshal(p)
}

func Deserialize(data string, p any) error {
	return json.Unmarshal(json.RawMessage(data), p)
}

func duplicate_message(data Message) bool {
	//check if message is duplicate
	//sha256 of data in strings
	msg_byte, _ := Serialize(data)
	msg_sha := sha256.Sum256(msg_byte)

	//check if message is duplicate
	tx_set.m.Lock()
	_, flag := tx_set.Tx_pool_set[string(msg_sha[:])]
	tx_set.m.Unlock()

	if flag {
		return flag
	}

	tx_set.m.Lock()
	tx_set.Tx_pool_set[string(msg_sha[:])] = true
	cur_size := len(tx_set.tx_pool)
	if cur_size == 100 {
		delete(tx_set.Tx_pool_set, tx_set.tx_pool[tx_set.cur_pointer])
		// store_in_map(string(tx_sha[:]))
		// tx_pool_set[string(tx_sha[:])] = true
		tx_set.tx_pool[tx_set.cur_pointer] = string(msg_sha[:])
		tx_set.cur_pointer = (tx_set.cur_pointer + 1) % 100
	} else {
		tx_set.tx_pool = append(tx_set.tx_pool, string(msg_sha[:]))
		// store_in_map(string(tx_sha[:]))
		tx_set.Tx_pool_set[string(msg_sha[:])] = true
	}
	tx_set.m.Unlock()

	return flag
}

// creating a socket
func create_socket() net.Conn {
	//client code
	conn, err := net.Dial(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT)
	if err != nil {
		fmt.Println("Error connecting to server", err)
	}
	return conn
}

// request gateway and storage node information from server
func request_gw(conn net.Conn) dh.Neighbours_GW {
	var err error
	_, err = conn.Write([]byte("GATEWAY_NODE"))
	if err != nil {
		fmt.Println("Error writing to socket")
	}
	// var err error
	// buffer := make([]byte, 1024)
	// _, err = conn.Read(buffer)
	var buffer []byte
	temp_buffer := make([]byte, 1024)
	for {
		n, e := conn.Read(temp_buffer)
		buffer = append(buffer, temp_buffer[:n]...)
		if n < 1024 || e == io.EOF {
			// fmt.Println("End line", string(buffer))
			break
		} else {
			// fmt.Println("Line going ", string(buffer))
			err = e
		}
		//clean temp_buffer
		temp_buffer = make([]byte, 1024)
	}
	if err != nil {
		fmt.Println("Error reading from socket")
	}
	fmt.Println("Received before clustering : ", string(buffer))
	//deserialize the message
	var data dh.Neighbours_GW
	_ = Deserialize(string(buffer), &data)
	// fmt.Println(data, e)
	conn.Close()
	return data
}

//listen for socket

func listen_socket() {
	// flag := <-p2pStartListening
	// fmt.Println("Listening to socket", flag)
	// if !(flag) {
	// 	return
	// }
	//server code
	ln, err := net.Listen(SERVER_TYPE, ":"+MY_PORT)
	if err != nil {
		fmt.Println("Error listening to socket")
	}
	fmt.Println("Listening to socket - ", MY_PORT)
	for {
		conn, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting connection")
		}
		rcv_ip := strings.Split(conn.RemoteAddr().String(), ":")[0]
		comp := strings.Compare(rcv_ip, SERVER_HOST)
		// fmt.Println("accepted connection from ip address: ", rcv_ip, SERVER_HOST, comp)
		if comp == 0 {
			//new neighbours received
			var err error
			var buffer []byte
			temp_buffer := make([]byte, 1024)
			for {
				n, e := conn.Read(temp_buffer)
				buffer = append(buffer, temp_buffer[:n]...)
				if n < 1024 || e == io.EOF {
					// fmt.Println("End line", string(buffer))
					break
				} else {
					// fmt.Println("Line going ", string(buffer))
					err = e
				}
				//clean temp_buffer
				temp_buffer = make([]byte, 102)
			}
			if err != nil {
				fmt.Println("Error reading from socket", err)
			}

			fmt.Println("Received after clustering : ", string(buffer), "from ip address of DN: ", strings.Split(conn.RemoteAddr().String(), ":")[0])
			//deserialize the message
			_ = Deserialize(string(buffer), &neighbours)
			ping_neighbours()
			time.Sleep(60 * time.Second)
		} else {
			go handle_connection(conn)
		}
	}
}

func handle_connection(conn net.Conn) {
	for {
		var err error
		var buffer []byte
		temp_buffer := make([]byte, 1024)
		for {
			n, e := conn.Read(temp_buffer)
			buffer = append(buffer, temp_buffer[:n]...)
			if n < 1024 || e == io.EOF {
				break
			} else {
				err = e
			}
			//clean temp_buffer
			temp_buffer = make([]byte, 1024)
		}
		// n, err := conn.Read(buffer)
		if err != nil {
			fmt.Println("Error reading from socket in handle connection", err)
			return
			//resolve return as per error
		}
		//spilt the buffer by @ into messages and handle each message
		messages := strings.Split(string(buffer), "@")
		for _, v := range messages {
			if v != "" {
				go handle_message(conn, []byte(v), len(v))
			}
		}
		// go handle_message(conn, buffer, n)
	}
}

func handleRecvVtx(msg Message) {
	var vtx dh.Vertex
	_ = dh.Deserialize(msg.Data, &vtx)
	hashOfTx := hex.EncodeToString(vtx.Tx.Hash[:])
	dag.Mux.Lock()
	_, flag := dag.Graph[hashOfTx]
	dag.Mux.Unlock()
	if flag {
		fmt.Println("Vertex already exists in the dag")
		return
	}
	fmt.Println("Received vertex from the network: ", hashOfTx)
	// add the vertex to the dag
	fmt.Println("Adding vertex to the dag: ", hashOfTx)

	dh.AddVtxToDAG(vtx, dag, gwPrivateKey)

	fmt.Println("Vertex added to the dag: ", hashOfTx)
	// broadcast the vertex to the other gateways/SN
	BroadcastMsg(msg)
}

func handleTxConfirmation(msg Message) {
	txHash := msg.Data
	// confirm the Tx
	dag.Mux.Lock()
	vertex, ok := dag.Graph[txHash]
	if ok && !vertex.Confirmed {
		vertex.Confirmed = true
		dag.Graph[txHash] = vertex
		dag.Mux.Unlock()
		fmt.Println("vertex confirmed: ", vertex.Vertex_no)

		// broadcast the vertex to the other gateways/SN
		BroadcastGW(msg, neighbours)
	} else if vertex.Confirmed {
		dag.Mux.Unlock()
	} else {
		//check orphan pool
		dag.Mux.Unlock()
		flag, vertex2 := dh.Ack_orphan_checker(txHash, dag)
		if flag {
			//add orphan to dag and broadcast
			dag.Mux.Lock()
			vertex2.Confirmed = true
			dag.Graph[txHash] = vertex2
			dag.Mux.Unlock()
			fmt.Println("vertex confirmed from orphan: ", vertex2.Vertex_no)
			// broadcast the vertex to the other gateways/SN
			BroadcastGW(msg, neighbours)
		}
	}

}

func handle_message(conn net.Conn, buffer []byte, n int) {
	//deserialize the message
	var msg Message
	_ = Deserialize(string(buffer), &msg)
	//clear the buffer
	buffer = nil
	if msg.Type == "PING" {
		var data GwMessage
		_ = Deserialize(string(msg.Data), &data)
		if data.Token == neighbours.Own_token_gw {
			//save connection with ip address key and connection as value
			connections[strings.Split(conn.RemoteAddr().String(), ":")[0]] = conn
			neighbours.Gw = append(neighbours.Gw, strings.Split(conn.RemoteAddr().String(), ":")[0])
			//save token of gateway with ip address key and token as value
			neighbours.Token_gw[strings.Split(conn.RemoteAddr().String(), ":")[0]] = data.Token
			fmt.Println("Ping received from gateway ", strings.Split(conn.RemoteAddr().String(), ":")[0])
		} else {
			fmt.Println("Transaction is invalid", data.Token, neighbours.Own_token_gw)
			// conn.Close()
		}
	} else if msg.Type == "GW_TRANSACTION" {
		// check if message is duplicate
		if duplicate_message(msg) {
			fmt.Println("Duplicate message received")
			fmt.Println("*************************************")
			return
		}
		var vtx dh.Vertex
		_ = dh.Deserialize(msg.Data, &vtx)
		fmt.Println("Vertex no received: ", vtx.Vertex_no)
		if vtx.Vertex_no == 0 && vtx.Gateway_no == 0 && vtx.Weight == 0 {
			hash := hex.EncodeToString(vtx.Tx.Hash[:])
			fmt.Println("Orphan Count: ", hash, conn.RemoteAddr().String(), msg.Type)
		}
		fmt.Println("Transaction received from other gateway", vtx.Gateway_no)
		handleRecvVtx(msg)
	} else if msg.Type == "ACK" {
		// check if message is duplicate
		fmt.Println("ACK received")
		if duplicate_message(msg) {
			fmt.Println("Duplicate message received of ACK")
			fmt.Println("*************************************")
			return
		}
		handleTxConfirmation(msg)
	} else if msg.Type == "DOWNLOAD_DAG" {
		// check if message is duplicate
		if duplicate_message(msg) {
			fmt.Println("Duplicate message received: DOWNLOAD")
			fmt.Println("*************************************")
			return
		}
		dag.Mux.Lock()
		for _, value := range dag.Graph {
			data_bytes, _ := dh.Serialize(value)
			// send the vertex to the other gateway
			var msg Message
			msg.Type = "DOWNLOAD_DAG_RESPONSE"
			msg.Data = string(data_bytes)
			Send(conn, msg)
		}
		dag.Mux.Unlock()
	} else if msg.Type == "DOWNLOAD_DAG_RESPONSE" {
		// check if message is duplicate
		if duplicate_message(msg) {
			fmt.Println("Duplicate message received: DOWNLOAD RESPONSE")
			return
		}

		// add the vertex to the dag
		var vtx dh.Vertex
		_ = dh.Deserialize(msg.Data, &vtx)
		fmt.Println("DAG download vertex from gateway", vtx.Gateway_no)
		fmt.Println("Vertex no downloaded: ", vtx.Vertex_no)
		if vtx.Vertex_no == 0 && vtx.Gateway_no == 0 && vtx.Weight == 0 {
			hash := hex.EncodeToString(vtx.Tx.Hash[:])
			fmt.Println("Orphan Count: ", hash, conn.RemoteAddr().String(), msg.Type)
		}
		dh.UploadToDAG(vtx, dag)
	} else {
		fmt.Println("Invalid message type", msg.Type)
	}
	fmt.Println("*************************************")
}

func Send(conn net.Conn, msg Message) {
	data_bytes, _ := Serialize(msg)
	//add @ to the start of the message
	data_bytes = append([]byte("@"), data_bytes...)
	// fmt.Println("sending data to gateway: ", conn.RemoteAddr().String())
	_, err := conn.Write(data_bytes)
	if err != nil {
		fmt.Println("Error writing to socket")
	}
}

func BroadcastGW(msg Message, temp_neighbours dh.Neighbours_GW) {

	data_bytes, _ := Serialize(msg)
	//add @ to the start of the message
	data_bytes = append([]byte("@"), data_bytes...)

	// broadcast to gateways
	for _, v := range temp_neighbours.Gw {
		//not to broadcast to my ip address
		if v != my_ipaddress {
			conn, flag := connections[v]
			if !flag {
				fmt.Println("Connection not found")
				continue
			}
			// fmt.Println("sending data to gateway: ", conn.RemoteAddr().String())
			_, err := conn.Write(data_bytes)
			if err != nil {
				fmt.Println("Error writing to socket", err)
			}
		}
	}
}

func Broadcast(msg Message, temp_neighbours dh.Neighbours_GW) {
	fmt.Println("Broadcasting message to gateways: ", temp_neighbours.Gw)
	data_bytes, _ := Serialize(msg)
	//add @ to the start of the message
	data_bytes = append([]byte("@"), data_bytes...)

	// broadcast to gateways
	for _, v := range temp_neighbours.Gw {
		//not to broadcast to my ip address
		if v != my_ipaddress {
			conn, flag := connections[v]
			if !flag {
				fmt.Println("Connection not found")
				continue
			}
			// fmt.Println("sending data to gateway: ", conn.RemoteAddr().String())
			_, err := conn.Write(data_bytes)
			if err != nil {
				fmt.Println("Error writing to socket")
			}
		}
	}

	var data_sn send_data_sn
	//initialize data_sn
	data_sn.Data = msg.Data
	data_sn.Own_token = ""
	data_sn.Own_nodeno = -1
	data_sn.Msg_no = -1
	data_sn.Msg_type = "GW_TRANSACTION"
	data_sn.Routing_token = ""
	data_sn.Own_token = temp_neighbours.Token_sn
	data_bytes2, _ := Serialize(data_sn)
	//add @ to the start of the message
	data_bytes2 = append([]byte("@"), data_bytes2...)
	fmt.Println("Broadcasting to SN: ", temp_neighbours.Sn)
	for _, v := range temp_neighbours.Sn {
		conn, flag := connections[v]

		if !flag {
			fmt.Println("SN: Connection not found")
			continue
		}
		_, err := conn.Write(data_bytes2)
		if err != nil {
			fmt.Println("Error writing to socket")
		}
	}
}

func BroadcastVtx(vertex dh.Vertex) {
	// serialize the vertex
	data_vtx, _ := Serialize(vertex)
	// create a message
	var msg Message
	msg.Type = "GW_TRANSACTION"
	msg.Data = string(data_vtx)
	msg.Timestamp = time.Now().UnixNano()
	// Broadcast to Gw
	Broadcast(msg, neighbours)
}

func BroadcastMsg(msg Message) {

	// Broadcast to Gw and Sn
	Broadcast(msg, neighbours)
}

func ping_neighbours() {
	var packet Message
	var data GwMessage
	data.Msg_type = "PING"
	for _, v := range neighbours.Gw {
		if v != my_ipaddress {
			_, flag := connections[v]
			if flag {
				// fmt.Println("Connection already exists with gateway ", v)
				continue
			}
			conn, err := net.Dial(SERVER_TYPE, v+":"+MY_PORT)
			if err != nil {
				fmt.Println("Error connecting to server " + v)
				fmt.Println(err)
				continue
				// return
			}
			data.Token = neighbours.Token_gw[v]
			//read on that connection
			go handle_connection(conn)
			//save connection
			connections[v] = conn
			data_bytes, _ := Serialize(data)
			//add @ to the start of the message

			// data_string := string(data_bytes)
			// // add \n to the end of the string
			// data_string = data_string + "\n"
			//convert string to byte array
			// data_bytes = []byte(data_string)
			// into packet
			packet.Type = data.Msg_type
			packet.Data = string(data_bytes)
			packet.Timestamp = time.Now().UnixNano()
			// serialize the packet
			data_packet_bytes, _ := Serialize(packet)
			data_packet_bytes = append([]byte("@"), data_packet_bytes...)
			_, err = conn.Write(data_packet_bytes)
			if err != nil {
				fmt.Println("Error writing to socket")
			}
		}
	}
	var data_sn send_data_sn
	//initialize data_sn
	data_sn.Data = ""
	data_sn.Own_token = ""
	data_sn.Own_nodeno = -1
	data_sn.Msg_no = -1
	data_sn.Msg_type = "GW_PING"
	data_sn.Routing_token = ""

	data_bytes, _ := Serialize(data_sn)
	data_bytes = append([]byte("@"), data_bytes...)
	for _, v := range neighbours.Sn {
		data_sn.Own_token = neighbours.Token_sn
		conn, err := net.Dial(SERVER_TYPE, v+":"+"8282")
		if err != nil {
			fmt.Println("Error connecting to server " + v)
			fmt.Println(err)
			continue
			// return
		}
		//save connection
		connections[v] = conn
		//listen on that connection
		go handle_connection(conn)
		_, err = conn.Write(data_bytes)
		if err != nil {
			fmt.Println("Error writing to socket")
		}
	}
}

func download_dag() {
	// no need to download dag if I am the first node
	if len(neighbours.Gw) == 0 {
		return
	}
	fmt.Println("Downloading dag")
	// send request a random GW
	var data Message
	data.Type = "DOWNLOAD_DAG"
	data.Timestamp = time.Now().UnixNano()
	data.Data = ""
	// get a random GW
	rand_in := rand.Intn(len(neighbours.Gw))
	gw := neighbours.Gw[rand_in]
	fmt.Println("Sending request to ", gw)
	var flag bool

	download_gw_conn, flag = connections[gw]
	if !flag {
		fmt.Println("Connection not found")
		return
	}

	data_bytes, _ := Serialize(data)
	data_bytes = append([]byte("@"), data_bytes...)
	_, err := download_gw_conn.Write(data_bytes)
	if err != nil {
		fmt.Println("Error writing to socket")
		return
	}
}

func Gateway(dag_t *dh.DAG, gwPrivateKey_t *ecdsa.PrivateKey, p2ptestx chan bool) {
	dag = dag_t
	gwPrivateKey = gwPrivateKey_t
	// fmt.Println("Hello World from gateway")
	connections = make(map[string]net.Conn)
	tx_set.m.Lock()
	tx_set.Tx_pool_set = make(map[string]bool)
	tx_set.m.Unlock()
	conn := create_socket()
	my_ipaddress = strings.Split(conn.LocalAddr().String(), ":")[0]
	// fmt.Println("My ip address is: ", strings.Split(conn.LocalAddr().String(), ":")[0])
	neighbours = request_gw(conn)
	dh.Neighbours_gw = neighbours
	go listen_socket()
	fmt.Println("Pinging neighbours")
	ping_neighbours()
	download_dag()

	fmt.Println("Gateway is ready to receive transactions")
	p2ptest := p2ptestx
	if neighbours.Own_nodeno%8 == 0 {
		fmt.Println("I am the generator node")
		p2ptest <- true
	} else {
		fmt.Println("I am not the generator node")
		p2ptest <- false
	}
	//error here
	fmt.Println("P2P code finished")
	for {
		time.Sleep(10000000)
	}
}
