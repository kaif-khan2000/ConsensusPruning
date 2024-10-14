package P2P_Manager

import (
	dh "StorageNode/Data_Handler"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
)

// discovery node address
const (
	SERVER_PORT = "8080"
	SERVER_HOST = "150.0.0.100"
	// SERVER_HOST = "localhost"
	SERVER_TYPE  = "tcp"
	MY_PORT      = "8282"
	MY_HOST      = ""
	tx_pool_size = 100
)

type Neighbours struct {
	Gn                  map[int]string // group nodes in the sn_cluster
	Adj                 map[int]string // adjacent nodes in the sn_cluster
	Aux                 map[int]string // auxiliary nodes in the sn_cluster
	Gn_order            []int          // order of group nodes
	Routing_nodes       []string       // routing nodes in the sn_cluster
	Routing_nodes_token []string       // routing nodes token in the sn_cluster
	SN_token            map[int]string // token of each node in the sn_cluster
	Own_token           string         // self token
	Own_nodeno          int            // self node number
	Own_cluster         int            // self cluster number
}

// message structure
type Message struct {
	Type      string
	Data      string
	Timestamp int64
}

type send_data struct {
	Own_token     string
	Own_nodeno    int
	Data          string
	Msg_type      string
	Msg_no        int
	Routing_token string
}

type con_info struct {
	Conn net.Conn
	Err  error
}

type tx_pool_set struct {
	Tx_pool_set map[string]bool
	m           sync.Mutex
}

var edges Neighbours
var nodes_in_cluster int
var myip_address string
var tx_pool []string
var tx_set tx_pool_set
var cur_pointer int
var connections map[int]con_info
var orphan_count int
var time1 time.Time

func init() {
	time1 = time.Now()
	fmt.Println("start of dh.")
}

var dag *dh.DAG

func Serialize(p interface{}) ([]byte, error) {
	return json.Marshal(p)
}

func Deserialize(data string, p any) error {
	return json.Unmarshal(json.RawMessage(data), p)
}

func create_socket() net.Conn {
	//dial discovery node
	conn, err := net.Dial(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT)
	if err != nil {
		fmt.Println("Error connecting to server " + SERVER_HOST)
		fmt.Println(err)
		// return
	}
	return (conn)
}

func request_DN(conn net.Conn) {
	//request to discovery node with flag STORAGE_NODE
	_, err := conn.Write([]byte("STORAGE_NODE"))
	if err != nil {
		fmt.Println("Error writing to socket")
	}
	// var err error
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
	fmt.Println("Received neighbours from DN: ", string(buffer))
	//store data into neighbours data struct with variable name edges
	_ = Deserialize(string(buffer), &edges)
	// fmt.Println(edges, e)
}

// func store_in_map(s string) {
// 	//store in map
// 	var m sync.Mutex
// 	m.Lock()
// 	tx_pool_set[s] = true
// 	m.Unlock()
// }

func duplicate_transaction(data dh.Transaction) bool {
	//check if transaction is duplicate
	//sha256 of data in string
	tx_byte, _ := Serialize(data)
	tx_sha := sha256.Sum256(tx_byte)
	tx_set.m.Lock()
	_, flag := tx_set.Tx_pool_set[string(tx_sha[:])]
	tx_set.m.Unlock()
	if flag {
		fmt.Println("Duplicate transaction", data)
		return false
	}
	cur_size := len(tx_pool)
	if cur_size == 100 {
		tx_set.m.Lock()
		delete(tx_set.Tx_pool_set, tx_pool[cur_pointer])
		tx_set.m.Unlock()
		tx_pool[cur_pointer] = string(tx_sha[:])
		cur_pointer = (cur_pointer + 1) % 100
		return true
	} else {
		tx_pool = append(tx_pool, string(tx_sha[:]))
		tx_set.m.Lock()
		tx_set.Tx_pool_set[string(tx_sha[:])] = true
		tx_set.m.Unlock()
		return true
	}
}

func ping_neighbours(data Neighbours) {
	//ping neighbours
	//ping adjacent nodes
	for node_no, v := range data.Adj {
		if v != "" {
			//ping ip address
			con, err := net.Dial(SERVER_TYPE, v+":"+MY_PORT)
			if err != nil {
				fmt.Println("Error connecting to server " + v)
				fmt.Println(err)
				continue
				// return
			}
			//save connection
			connections[node_no] = con_info{con, err}
			go read_connection(con)
			vtx := ""
			data, _ := Serialize(send_data{data.Own_token, data.Own_nodeno, vtx, "PING", -1, ""})
			//add @ to the start of the message
			data = append([]byte("@"), data...)
			_, err = con.Write([]byte(data))
			if err != nil {
				fmt.Println("Error writing to socket")
			}
		}
	}
	//ping group nodes
	for node_no, v := range data.Gn {
		// if v != "" && node_no != data.Gn_order[0] {
		if v != "" {
			fmt.Println("Ping to group nodes", v)
			//ping ip address
			con, err := net.Dial(SERVER_TYPE, v+":"+MY_PORT)
			if err != nil {
				fmt.Println("Error connecting to server " + v)
				fmt.Println(err)
				continue
				// return
			}
			connections[node_no] = con_info{con, err}
			go read_connection(con)
			data, _ := Serialize(send_data{data.Own_token, data.Own_nodeno, "", "PING", -1, ""})
			//add @ to the start of the message
			data = append([]byte("@"), data...)
			_, err = con.Write([]byte(data))
			if err != nil {
				fmt.Println("Error writing to socket")
			}
		}
	}
	//ping auxillary nodes
	for node_no, v := range data.Aux {
		if v != "" {
			//ping ip address
			con, err := net.Dial(SERVER_TYPE, v+":"+MY_PORT)
			if err != nil {
				fmt.Println("Error connecting to server " + v)
				fmt.Println(err)
				continue
			}
			connections[node_no] = con_info{con, err}
			go read_connection(con)

			data, _ := Serialize(send_data{data.Own_token, data.Own_nodeno, "", "PING", -1, ""})
			//add @ to the start of the message
			data = append([]byte("@"), data...)
			_, err = con.Write([]byte(data))
			if err != nil {
				fmt.Println("Error writing to socket")
			}
		}
	}
	//ping routing nodes
	for node_no, v := range data.Routing_nodes {
		if v != "" {
			//ping ip address
			con, err := net.Dial(SERVER_TYPE, v+":"+MY_PORT)
			if err != nil {
				fmt.Println("Error connecting to server " + v)
				fmt.Println(err)
				continue
				// return
			}
			connections[nodes_in_cluster+node_no] = con_info{con, err}
			go read_connection(con)
			data, _ := Serialize(send_data{data.Routing_nodes_token[node_no], data.Own_nodeno, "", "ROUTING_PING", -1, data.Own_token})
			//add @ to the start of the message
			data = append([]byte("@"), data...)
			_, err = con.Write([]byte(data))
			if err != nil {
				fmt.Println("Error writing to socket")
			}
		}
	}
}

func listen_socket() {
	//listen other SN on my ipaddress and on port MY_PORT
	ln, err := net.Listen(SERVER_TYPE, myip_address+":"+MY_PORT)
	if err != nil {
		fmt.Println("Error listening to socket")
	}
	count := 0
	//continously accept multiple connections
	for {
		conn_listen, err := ln.Accept()
		if err != nil {
			fmt.Println("Error accepting connection")
		}
		//continously accept connection and continuosly read on each connection
		go read_connection(conn_listen)
		count++
	}
}

func read_connection(conn net.Conn) {
	//continously read on 1 connection
	for {
		//handle chord_harrary
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
		if err != nil {
			continue
		}
		messages := strings.Split(string(buffer), "@")
		for _, v := range messages {
			if v != "" {
				go handle_connection(conn, []byte(v), len(v))
			}
		}

	}
}

func handle_connection(conn net.Conn, buffer []byte, n int) {
	//store data into neighbours
	var rcv_data send_data
	err := Deserialize(string(buffer), &rcv_data)
	fmt.Println("Received data: ", rcv_data, conn.RemoteAddr().String())
	if err != nil {
		fmt.Println("Error in deserializing: ", err, string(buffer))
	}
	//if sn send packet with wrong token then close connection
	//HANDLING ROUTING NODES IS PROBLEM PLEASE SOLVE IT
	handle_message(rcv_data, conn, err)
	// if rcv_data.Own_token == edges.SN_token[rcv_data.Own_nodeno] || ((rcv_data.Msg_type == "ROUTING_PING" || rcv_data.Msg_type == "ROUTING") && rcv_data.Own_token == edges.Own_token) || ((rcv_data.Msg_type == "GW_PING" || rcv_data.Msg_type == "GW_TRANSACTION") && rcv_data.Own_token == edges.Own_token) {
	// 	handle_message(rcv_data, conn, err)
	// } else {
	// 	fmt.Println("Error in ping wrong token", rcv_data, edges.Own_token)
	// 	conn.Close()
	// }
}

func gn_broadcast(rcv_data send_data) {
	//log n base 2
	//broadcast transaction to group nodes according to gn_order
	cur_msg_no := rcv_data.Msg_no
	for i := cur_msg_no + 1; i < len(edges.Gn_order); i++ {
		//broadcast transaction to edges.Gn[i]
		// print connections to whom sending data
		fmt.Println("SN gossip transaction send neighbors", edges.Gn_order[i])
		Con, flag := connections[edges.Gn_order[i]]

		if !flag {
			fmt.Println("Node not present till yet")
		} else {
			con := Con.Conn
			// print connections to whom sending data
			fmt.Println("SN gossip message send ", edges.Gn_order[i])
			data_bytes, _ := Serialize(send_data{edges.Own_token, edges.Own_nodeno, rcv_data.Data, "TRANSACTION", i, ""})
			//add @ to the start of the message
			data_bytes = append([]byte("@"), data_bytes...)
			_, err_gn := con.Write([]byte(data_bytes))
			if err_gn != nil {
				fmt.Println("Error writing to socket")
			}
		}
	}
}

func adj_broadcast(rcv_data send_data) {
	//now send to adjacent nodes
	//broadcast transaction to adjacent nodes
	for i := range edges.Adj {
		// fmt.Println("message send ", i)
		if i == rcv_data.Own_nodeno {
			continue
		}
		Con, flag := connections[i]

		if !flag {
			fmt.Println("Node not present till yet")
		} else {
			con := Con.Conn
			vtx := rcv_data.Data
			data, _ := Serialize(send_data{edges.Own_token, edges.Own_nodeno, vtx, "TRANSACTION", rcv_data.Msg_no + 1, ""})
			//add @ to the start of the message
			data = append([]byte("@"), data...)
			_, err_adj := con.Write([]byte(data))
			if err_adj != nil {
				fmt.Println("Error writing to socket")
			}
		}
	}
}

func routing_broadcast(rcv_data send_data, rcv_from_ip string) {
	//send to routing node if present
	if len(edges.Routing_nodes) > 0 {
		//broadcast transaction to routing nodes
		for i, j := range edges.Routing_nodes {
			if j != "" && j != myip_address && j != rcv_from_ip {
				//ping ip address
				con, flag := connections[nodes_in_cluster+i]
				if !flag {
					fmt.Println("Node not present till yet")
					continue
				}
				if con.Err != nil {
					fmt.Println("Error connecting to server " + j)
					fmt.Println(con.Err)
					continue
					// return
				}
				Con := con.Conn
				msg_no := 0
				data_bytes, _ := Serialize(send_data{edges.Routing_nodes_token[i], edges.Own_nodeno, rcv_data.Data, "ROUTING", msg_no, edges.Own_token})
				//add @ to the start of the message
				data_bytes = append([]byte("@"), data_bytes...)
				_, err := Con.Write([]byte(data_bytes))
				if err != nil {
					fmt.Println("Error writing to socket")
				}
			}
		}
	}
}

func Send_ACK(conn net.Conn, data string) {
	fmt.Println("Sending ACK: to ", strings.Split(conn.RemoteAddr().String(), ":")[0], data)
	msg := Message{
		Type: "ACK",
		Data: string(data),
		// Timestamp: time.Now().Unix(),
	}
	data_bytes, _ := Serialize(msg)
	data_bytes = append([]byte("@"), data_bytes...)
	fmt.Println("Sending ACK to gateway: ", string(data_bytes))
	_, err := conn.Write([]byte(data_bytes))
	if err != nil {
		fmt.Println("Error writing to socket")
	}
}

func handleTransactions(vtx dh.Vertex) bool {
	// add vertex to the blockchain
	ok, err := dh.VerifyVertex(vtx, dag)
	if ok {
		dh.AddToDAG(vtx, dag)
		// p2p.Send(conn, vtx.Tx.Hash[:])
		fmt.Println("Received vertex: ", vtx)
		return true
	} else {
		orphan_count += 1
		fmt.Println("Orphan count: ", time.Since(time1), orphan_count, err)
		return false
	}
}

func handle_message(rcv_data send_data, conn net.Conn, err error) {
	// rcv_from_ip := strings.Split(conn.RemoteAddr().String(), ":")[0]
	if rcv_data.Msg_type == "PING" {
		edges.Gn[rcv_data.Own_nodeno] = strings.Split(conn.RemoteAddr().String(), ":")[0]
		// add in gn_order
		edges.Gn_order = append(edges.Gn_order, rcv_data.Own_nodeno)
		connections[rcv_data.Own_nodeno] = con_info{conn, err}
		// conntected to node
		fmt.Println("Ping from SN ", rcv_data.Own_nodeno, rcv_data.Own_token)
		// var type string
		// _, flag_adj := edges.Adj[rcv_data.Own_nodeno]
		// if flag_adj {
		// 	edges.Adj[rcv_data.Own_nodeno] = strings.Split(conn.RemoteAddr().String(), ":")[0]
		// 	connections[rcv_data.Own_nodeno] = con_info{conn, err}
		// 	// reading is going on store connection for sending
		// } else {
		// 	_, flag_gn := edges.Gn[rcv_data.Own_nodeno]
		// 	if flag_gn {
		// 		edges.Gn[rcv_data.Own_nodeno] = strings.Split(conn.RemoteAddr().String(), ":")[0]
		// 		connections[rcv_data.Own_nodeno] = con_info{conn, err}
		// 		// reading is going on store connection for sending

		// 	} else {
		// 		_, flag_aux := edges.Aux[rcv_data.Own_nodeno]
		// 		if edges.SN_token[rcv_data.Own_nodeno] == rcv_data.Own_token && flag_aux {
		// 		} else {
		// 			fmt.Println("Error in ping wrong node no.", rcv_data, edges)
		// 		}
		// 	}
		// }
	} else if rcv_data.Msg_type == "TRANSACTION" {
		fmt.Println("Transaction from SN ", rcv_data.Data)
		var vtx dh.Vertex
		err = Deserialize(rcv_data.Data, &vtx)
		if err != nil {
			fmt.Println("Error in deserializing transaction - p2p sn")
		}

		if !duplicate_transaction(vtx.Tx) {
			fmt.Println("This is duplicate packet from routing", rcv_data)
		} else {
			fmt.Println("save transaction in transaction pool and broadcast")
			if !handleTransactions(vtx) {
				fmt.Println("Error in handling transaction invalid")
				return
			}
			gn_broadcast(rcv_data)
			// if rcv_data.Msg_no < int(math.Logb(float64(nodes_in_cluster)))-3 {
			// 	gn_broadcast(rcv_data)
			// 	adj_broadcast(rcv_data)
			// 	routing_broadcast(rcv_data, rcv_from_ip)
			// } else if rcv_data.Msg_no < int(math.Logb(float64(nodes_in_cluster))) {
			// 	adj_broadcast(rcv_data)
			// 	routing_broadcast(rcv_data, rcv_from_ip)
			// 	// 	//duplicate here
			// } else {
			// 	//send to routing node if present
			// 	routing_broadcast(rcv_data, rcv_from_ip)
			// }
		}

	} else if rcv_data.Msg_type == "ROUTING" {
		fmt.Println("Transaction from Routing ", rcv_data.Data)
		var vtx dh.Vertex
		err = Deserialize(rcv_data.Data, &vtx)
		if err != nil {
			fmt.Println("Error in deserializing transaction - p2p sn")
		}

		if !duplicate_transaction(vtx.Tx) {
			fmt.Println("This is duplicate packet from routing", rcv_data)
		} else {
			fmt.Println("save transaction in transaction pool and broadcast")
			if !handleTransactions(vtx) {
				fmt.Println("Error in handling transaction invalid")
				return
			}
			rcv_from_ip := strings.Split(string(conn.RemoteAddr().String()), ":")[0]
			broadcast_transaction(rcv_data.Data, rcv_from_ip)
		}

		//packet received from routing node and now broadcast in own cluster
	} else if rcv_data.Msg_type == "ROUTING_PING" {
		connections[nodes_in_cluster+len(edges.Routing_nodes)] = con_info{conn, err}
		edges.Routing_nodes = append(edges.Routing_nodes, strings.Split(string(conn.RemoteAddr().String()), ":")[0])
		edges.Routing_nodes_token = append(edges.Routing_nodes_token, rcv_data.Routing_token)

	} else if rcv_data.Msg_type == "GW_PING" {

		fmt.Println("Ping from gateway")

	} else if rcv_data.Msg_type == "GW_TRANSACTION" {
		fmt.Println("Transaction from gateway ", rcv_data.Data)
		var vtx dh.Vertex
		err = Deserialize(rcv_data.Data, &vtx)
		if err != nil {
			fmt.Println("Error in deserializing transaction - p2p sn")
		}

		if !duplicate_transaction(vtx.Tx) {
			fmt.Println("This is duplicate packet from gateway", rcv_data)
		} else {
			fmt.Println("Transaction is ", rcv_data)
			fmt.Println("send tx to neighbours", edges)
			fmt.Println("save transaction in transaction pool and broadcast")
			if !handleTransactions(vtx) {
				fmt.Println("Error in handling transaction invalid")
				return
			}
			//broadcast transaction
			broadcast_transaction(rcv_data.Data, myip_address)
			// send ack to gateway
			data := hex.EncodeToString(vtx.Tx.Hash[:])
			Send_ACK(conn, data)

		}
	} else {
		fmt.Println("Error in message type", rcv_data, rcv_data.Msg_type)
	}
}

func broadcast_transaction(tx_brod string, rcv_from_ip string) {
	//broadcast transaction to group nodes according to gn_order
	var tx_packet send_data
	tx_packet.Msg_type = "TRANSACTION"
	tx_packet.Data = tx_brod
	tx_packet.Own_nodeno = edges.Own_nodeno
	tx_packet.Own_token = edges.Own_token
	tx_packet.Msg_no = -1
	tx_packet.Routing_token = ""
	gn_broadcast(tx_packet)
	tx_packet.Msg_no = len(edges.Gn)
	adj_broadcast(tx_packet)
	tx_packet.Msg_no = 0
	routing_broadcast(tx_packet, "")
}

func StorageNode(dag_t *dh.DAG) {
	dag = dag_t
	//start of main function
	//tx_pool_set is map of transactions which are incoming
	tx_set.m.Lock()
	tx_set.Tx_pool_set = make(map[string]bool)
	tx_set.m.Unlock()
	//connections is map of connections with other nodes
	connections = make(map[int]con_info)
	//fixed nodes in cluster
	nodes_in_cluster = 16
	//cur_pointer is pointer in tx_pool_set to delete old transactions
	cur_pointer = 0
	//create socket
	conn := create_socket()
	//get my ip address
	myip_address = strings.Split(string(conn.LocalAddr().String()), ":")[0]
	//request DN and get neighbours node number and token
	request_DN(conn)
	//can close connection with DN
	conn.Close()
	//listen socket
	go listen_socket()
	//sleep for 60 seconds
	// time.Sleep(20 * time.Second)

	//ping neighbours for connection establishment
	ping_neighbours(edges)

	//sleep for sometime so that n/w becomes stable
	// time.Sleep(30 * time.Second)

	for {
		// fmt.Println("Just not exiting the code")
		// wait for chord_harrary
	}
}
