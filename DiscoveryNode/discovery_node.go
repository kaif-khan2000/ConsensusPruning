package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"net"
	"strings"
	"time"

	"github.com/google/uuid"
)

// self info of Discovery Node
const (
	SERVER_PORT = "8080"
	SERVER_HOST = "150.0.0.100"
	// SERVER_HOST = "localhost"
	SERVER_TYPE = "tcp"
)

type Neighbours_SN struct {
	Gn                  map[int]string // Group Nodes
	Adj                 map[int]string // Adjacent Nodes
	Aux                 map[int]string // Auxiliary Nodes
	Gn_order            []int          // Group Nodes order (what is the order of the broadcasting of the group nodes)
	Routing_nodes       []string       // Routing nodes
	Routing_nodes_token []string       // Routing nodes token
	SN_token            map[int]string // Token of the StorageNode
	Own_token           string         // Token of the one which requested the Neigbours
	Own_nodeno          int            // Node number of the one which requested the Neigbours
	Own_cluster         int            // Cluster number of the one which requested the Neigbours
}

type Neighbours_GW struct {
	Gw           []string          // ip address of gateway nodes
	Sn           []string          // ip address of storage nodes
	Token_gw     map[string]string // Tokens of the GatewayNode
	Token_sn     string            // Token of the StorageNode
	Own_token_gw string            // Token of the one which requested the Neigbours
	Own_nodeno   int               // Number of node before it
}

// required for chord_harrary
var sn_cluster []map[int]string  // array of map of clusters (each map contains the Storage Nodes of a cluster)
var token_sn []map[int]string    // array of map of tokens (each map contains the tokens of the Storage Nodes of a cluster)
var routing []string             // array of routing nodes of each cluster
var routing_token_sn []string    // array of routing node's token of each cluster
var cluster_size_sn int = 16     // number of nodes in a cluster
var clustering_flag bool = false // flag to check if clustering is in progress or not
var total_sn int = 0             // number of storage nodes

// required for gw_clustering
var gw_cluster []map[int]string // array of map of clusters (each map contains the Gateway Nodes of a cluster)
var token_gw []map[int]string   // array of map of tokens (each map contains the tokens of the Gateway Nodes of a cluster)
var cluster_size_gw int = 8     // number of nodes in a gw_cluster
var no_of_gw int = -1           // number of gateway nodes

// serialize and deserialize
func Serialize(p interface{}) ([]byte, error) {
	return json.Marshal(p)
}

func Deserialize(data string, p interface{}) error {
	return json.Unmarshal(json.RawMessage(data), &p)
}

// create a socket and listen for incoming connections and accepts them
func create_socket() {
	ln, err := net.Listen(SERVER_TYPE, SERVER_HOST+":"+SERVER_PORT)
	if err != nil {
		// handle error
		fmt.Println("Error creating socket")
	}
	for {
		conn, err := ln.Accept()
		if err != nil {
			// handle error
			fmt.Println("Error accepting connection")
		}
		go handleConnection(conn)
	}
}

// handle the connection based on what the client sends
func handleConnection(conn net.Conn) {
	// do something
	buffer := make([]byte, 1024)
	n, err := conn.Read(buffer)
	if err != nil {
		fmt.Println("Error reading from socket")
	}
	fmt.Println("Received: ", string(buffer[:n]))

	if string(buffer[:n]) == "GATEWAY_NODE" {
		// handle the connection of GW
		fmt.Println("hello from gateway node :", conn.RemoteAddr().String())
		for clustering_flag {
			fmt.Println("clustering in progress")
			time.Sleep(1 * time.Second)
		}
		data := get_gateways(conn)
		fmt.Println("Data before serialize : ", data)
		//json data to byte
		gws, _ := Serialize(data)
		fmt.Println("Data: ", string(gws))
		_, err = conn.Write(gws)
		if err != nil {
			fmt.Println("Error writing to socket")
		}
		return
	} else if string(buffer[:n]) == "STORAGE_NODE" {
		// handle the connection of SN
		fmt.Println(conn.RemoteAddr().String())
		// increase the total number of storage nodes
		total_sn++
		data := chord_harrary(conn.RemoteAddr().String())
		fmt.Println("Data before serialize : ", data)
		//json data to byte
		nodes, _ := Serialize(data)
		fmt.Println("Data: ", string(nodes))
		_, err = conn.Write(nodes)
		if err != nil {
			fmt.Println("Error writing to socket")
		}

	} else {
		fmt.Println("Error: not a valid flag")
		return
	}

	conn.Close()
}

// creates a cluster of sn_nodes and generates tokens for each node
func create_cluster() {
	last := len(token_sn)
	sn_cluster = append(sn_cluster, make(map[int]string))
	token_sn = append(token_sn, make(map[int]string))
	for i := 0; i < cluster_size_sn; i++ {
		//sha256 of uuid of node
		token_sn[last][i] = uuid.New().String()
	}
}

// it will add an SN to the cluster and returns its neighbours according to chord_harrary
func chord_harrary(socket string) Neighbours_SN {
	//map new SN to node in chord_harrary
	var data Neighbours_SN
	//initializing data
	data.Adj = make(map[int]string)
	data.Aux = make(map[int]string)
	data.Gn = make(map[int]string)

	// only 1 cluster is present name 0
	cluster_index := 0
	// current node number
	node := total_sn
	sn_cluster[cluster_index][node] = strings.Split(socket, ":")[0]
	// create a token for the new node
	token_sn[cluster_index][node] = uuid.New().String()
	// own token
	data.Own_token = token_sn[cluster_index][node]
	// own node number
	data.Own_nodeno = node
	// own cluster number
	data.Own_cluster = cluster_index

	//initialize the data
	data.Gn = make(map[int]string)
	data.Adj = make(map[int]string)
	data.Aux = make(map[int]string)
	data.SN_token = make(map[int]string)

	// Gn nodes are 3 random nodes from the cluster except the current node
	r := rand.Perm(total_sn)
	// min of 3 and total_sn
	for i := 0; i < 3 && i < total_sn; i++ {
		if r[i] == node {
			continue
		}
		// print the current random node
		fmt.Println("random node: ", r[i])

		// if not node there then add it
		if sn_cluster[cluster_index][r[i]] != "" {
			// print that node is present
			fmt.Println("node is present")
			data.Gn[r[i]] = sn_cluster[cluster_index][r[i]]
			data.SN_token[r[i]] = token_sn[cluster_index][r[i]]
			// give gn_order
			data.Gn_order = append(data.Gn_order, r[i])
		}
	}

	// cur := node - 1
	// if cur < 0 {
	// 	cur = n - 1
	// }
	// adj1, flag1 := sn_cluster[cluster_index][cur]
	// if flag1 {
	// 	//if node-1 is present in cluster then provide its socket
	// 	data.Adj[cur] = adj1
	// } else {
	// 	//if node-1 is not present in cluster then provide its token_sn
	// 	data.Adj[cur] = ""
	// }
	// data.SN_token[cur] = token_sn[cluster_index][cur]
	// cur = node + 1
	// if cur >= n {
	// 	cur = 0
	// }
	// adj2, flag2 := sn_cluster[cluster_index][cur]
	// if flag2 {
	// 	//if node+1 is present in cluster then provide its socket
	// 	data.Adj[cur] = adj2
	// } else {
	// 	//if node+1 is not present in cluster then provide its token_sn
	// 	data.Adj[cur] = ""
	// }
	// data.SN_token[cur] = token_sn[cluster_index][cur]

	// temp := n
	// for temp > 4 {
	// 	cur = (node + (temp / 2)) % n
	// 	data.Gn_order = append(data.Gn_order, cur)
	// 	gn1, flag3 := sn_cluster[cluster_index][cur]
	// 	if flag3 {
	// 		data.Gn[cur] = gn1
	// 	} else {
	// 		data.Gn[cur] = ""
	// 	}
	// 	data.SN_token[cur] = token_sn[cluster_index][cur]
	// 	temp = temp / 2
	// }

	// temp = n
	// for temp > 4 {
	// 	cur = (node - (temp / 2)) % n
	// 	if cur < 0 {
	// 		cur = n + cur
	// 	}
	// 	aux1, flag3 := sn_cluster[cluster_index][cur]
	// 	if flag3 {
	// 		data.Aux[cur] = aux1
	// 	} else {
	// 		data.Aux[cur] = ""
	// 	}
	// 	data.SN_token[cur] = token_sn[cluster_index][cur]
	// 	temp = temp / 2
	// }
	// data.Own_token = token_sn[cluster_index][node]
	// data.Own_nodeno = node
	// data.Own_cluster = cluster_index
	return data
}

// assign a cluster to a gateway and returns its neighbours
func assign_gateway(cluster int, newgw_ip string) Neighbours_GW {

	var data Neighbours_GW
	data.Token_gw = make(map[string]string)
	//randomly select three unique numbers from 0 to len(gw_cluster[cluster])
	r := rand.Perm(len(gw_cluster[cluster]))

	// for i := 0; i < len(gw_cluster[cluster]); i++ {
	// 	a := r[i]
	// 	data.Gw = append(data.Gw, gw_cluster[cluster][a])
	// 	data.Token_gw[gw_cluster[cluster][a]] = token_gw[cluster][a]
	// }
	// _, flag := data.Token_gw[newgw_ip]
	// if flag {
	// 	data.Own_token_gw = data.Token_gw[newgw_ip]
	// 	fmt.Println("using old token of gateway: ", newgw_ip, " as: ", data.Own_token_gw, " in cluster: ", cluster, " for new gateway: ", newgw_ip)
	// } else {
	// 	data.Own_token_gw = ""
	// }

	if len(gw_cluster[cluster]) == 1 {
		// if only one gateway is present in cluster
		a := r[0]
		data.Gw = append(data.Gw, gw_cluster[cluster][a])
		data.Token_gw[gw_cluster[cluster][a]] = token_gw[cluster][a]
		_, flag := data.Token_gw[newgw_ip]
		if flag {
			data.Own_token_gw = data.Token_gw[newgw_ip]
			fmt.Println("using old token of gateway: ", newgw_ip, " as: ", data.Own_token_gw, " in cluster: ", cluster, " for new gateway: ", newgw_ip)
		} else {
			data.Own_token_gw = ""
		}
	} else if len(gw_cluster[cluster]) == 2 {
		// if only 2 gateway is present in cluster
		a, b := r[0], r[1]
		data.Gw = append(data.Gw, gw_cluster[cluster][a])
		data.Token_gw[gw_cluster[cluster][a]] = token_gw[cluster][a]
		data.Gw = append(data.Gw, gw_cluster[cluster][b])
		data.Token_gw[gw_cluster[cluster][b]] = token_gw[cluster][b]
		_, flag := data.Token_gw[newgw_ip]
		if flag {
			data.Own_token_gw = data.Token_gw[newgw_ip]
			fmt.Println("using old token of gateway: ", newgw_ip, " as: ", data.Own_token_gw, " in cluster: ", cluster, " for new gateway: ", newgw_ip)
		} else {
			data.Own_token_gw = ""
		}
	} else {
		a, b, c := r[0], r[1], r[2]
		data.Gw = append(data.Gw, gw_cluster[cluster][a])
		data.Token_gw[gw_cluster[cluster][a]] = token_gw[cluster][a]
		data.Gw = append(data.Gw, gw_cluster[cluster][b])
		data.Token_gw[gw_cluster[cluster][b]] = token_gw[cluster][b]
		data.Gw = append(data.Gw, gw_cluster[cluster][c])
		data.Token_gw[gw_cluster[cluster][c]] = token_gw[cluster][c]
		_, flag := data.Token_gw[newgw_ip]
		if flag {
			data.Own_token_gw = data.Token_gw[newgw_ip]
			fmt.Println("using old token of gateway: ", newgw_ip, " as: ", data.Own_token_gw, " in cluster: ", cluster, " for new gateway: ", newgw_ip)
		} else {
			data.Own_token_gw = ""
		}
	}
	// One storage Node as its neighbour
	temp_r := rand.Intn(4)
	if temp_r == 0 {
		//randomly select 1 SN from cluster
		r_c := rand.Intn(len(sn_cluster))
		r := rand.Intn(len(sn_cluster[r_c]))
		data.Sn = append(data.Sn, sn_cluster[r_c][r])
		data.Token_sn = token_sn[r_c][r]
	} else {
		data.Token_sn = ""
	}
	return data
}

// split the cluster
func split_cluster(cluster int) {
	new_gwlist := rand.Perm(len(gw_cluster[cluster]))
	fmt.Println("randomly arranged nodes are: ", new_gwlist)
	gw_cluster = append(gw_cluster, make(map[int]string))
	token_gw = append(token_gw, make(map[int]string))
	var new_cluster_size int = len(gw_cluster[cluster]) / 2
	//new cluster
	for i := 0; i < new_cluster_size; i++ {
		gw_cluster[len(gw_cluster)-1][i] = gw_cluster[cluster][new_gwlist[i]]
		token_gw[len(gw_cluster)-1][i] = uuid.New().String()
		delete(gw_cluster[cluster], new_gwlist[i])
		delete(token_gw[cluster], new_gwlist[i])
	}
	//adjust old cluster
	temp_cluster_gw := make(map[int]string)
	temp_cluster_token := make(map[int]string)
	i := 0
	for _, v := range gw_cluster[cluster] {
		temp_cluster_gw[i] = v
		temp_cluster_token[i] = uuid.New().String()
		i++
	}
	//add new gw in old cluster
	gw_cluster[cluster] = temp_cluster_gw
	token_gw[cluster] = temp_cluster_token
	fmt.Println("After split cluster is ", gw_cluster, token_gw)
	new_cluster := len(gw_cluster) - 1
	old_cluster := cluster
	//inform gw of old cluster
	for i, v := range gw_cluster[old_cluster] {
		conn, err := net.Dial("tcp", v+":4242")
		if err != nil {
			fmt.Println("error in dialing to gw")
			continue
		}
		data := assign_gateway(cluster, v)
		// atleast 1 SN in the cluster
		// assign 1st GW to SN
		if i == 0 {
			r_c := rand.Intn(len(sn_cluster))
			r := rand.Intn(len(sn_cluster[r_c]))
			data.Sn = append(data.Sn, sn_cluster[r_c][r])
			data.Token_sn = token_sn[r_c][r]
		}
		if data.Own_token_gw == "" {
			for i, v1 := range gw_cluster[old_cluster] {
				if v1 != v {
					data.Own_token_gw = token_gw[old_cluster][i]
					break
				}
			}
		} else {
			fmt.Println("gw of ip ", v, " has token ", data.Own_token_gw)
		}
		//send data
		data_bytes, _ := Serialize(data)
		_, err = conn.Write(data_bytes)
		if err != nil {
			fmt.Println("error in sending data to gw")
		}
		conn.Close()
	}
	//inform gw of new cluster
	for i, v := range gw_cluster[new_cluster] {
		conn, err := net.Dial("tcp", v+":4242")
		if err != nil {
			fmt.Println("error in dialing to gw")
			continue
		}
		data := assign_gateway(new_cluster, v)
		// atleast 1 SN in the cluster
		// assign 1st GW to SN
		if i == 0 {
			r_c := rand.Intn(len(sn_cluster))
			r := rand.Intn(len(sn_cluster[r_c]))
			data.Sn = append(data.Sn, sn_cluster[r_c][r])
			data.Token_sn = token_sn[r_c][r]
		}
		if data.Own_token_gw == "" {
			for i, v1 := range gw_cluster[old_cluster] {
				if v1 != v {
					data.Own_token_gw = token_gw[old_cluster][i]
					break
				}
			}
		} else {
			fmt.Println("gw of ip ", v, " has token ", data.Own_token_gw)
		}
		//send data
		data_bytes, _ := Serialize(data)
		_, err = conn.Write(data_bytes)
		if err != nil {
			fmt.Println("error in sending data to gw")
		}
		conn.Close()
	}
}

// returns neighbours of gateway
func get_gateways(conn net.Conn) Neighbours_GW {
	//if no cluster then create one and add Gw
	no_of_gw++
	new_gw := strings.Split(conn.RemoteAddr().String(), ":")[0]
	if len(gw_cluster) == 0 {
		gw_cluster = append(gw_cluster, make(map[int]string))
		token_gw = append(token_gw, make(map[int]string))
		gw_cluster[0][0] = new_gw
		token_gw[0][0] = uuid.New().String()
		var data Neighbours_GW
		data.Token_gw = make(map[string]string)
		data.Own_nodeno = 0
		//randomly select 1 SN from cluster
		r_c := rand.Intn(len(sn_cluster))
		r := rand.Intn(len(sn_cluster[r_c]))
		data.Sn = append(data.Sn, sn_cluster[r_c][r])
		data.Token_sn = token_sn[r_c][r]
		data.Own_token_gw = token_gw[0][0]
		fmt.Println("gw token is ", data.Own_token_gw)
		return data
	} else {
		//randomly select cluster
		cluster := rand.Intn(len(gw_cluster))
		if len(gw_cluster[cluster]) == 0 {
			//empty cluster do add node ip address
			gw_cluster[cluster][0] = new_gw
			//add token
			token_gw[cluster][0] = uuid.New().String()
			var data Neighbours_GW
			data.Token_gw = make(map[string]string)
			//randomly select 1 SN from cluster
			r_c := rand.Intn(len(sn_cluster))
			r := rand.Intn(len(sn_cluster[r_c]))
			data.Sn = append(data.Sn, sn_cluster[r_c][r])
			data.Token_sn = token_sn[r_c][r]
			data.Own_token_gw = token_gw[cluster][0]
			data.Own_nodeno = no_of_gw
			return data
		} else if len(gw_cluster[cluster]) < cluster_size_gw {
			gw_conn := assign_gateway(cluster, new_gw)
			//cluster is not full so add in cluster
			gw_cluster[cluster][len(gw_cluster[cluster])] = new_gw
			//add token
			token_gw[cluster][len(token_gw[cluster])] = uuid.New().String()
			gw_conn.Own_token_gw = token_gw[cluster][len(token_gw[cluster])-1]
			gw_conn.Own_nodeno = no_of_gw
			return gw_conn
		} else {
			fmt.Println("splitting cluster")
			fmt.Println("before clustering func gw_cluster is ", gw_cluster)
			split_cluster(cluster)
			clustering_flag = true
			time.Sleep(10 * time.Second)
			clustering_flag = false
			//print all clusters
			fmt.Println("after clustering func gw_cluster is ", gw_cluster)
			gw_conn := assign_gateway(cluster, new_gw)
			//cluster is not full so add in cluster
			gw_cluster[cluster][len(gw_cluster[cluster])] = new_gw
			//add token
			token_gw[cluster][len(token_gw[cluster])] = uuid.New().String()
			gw_conn.Own_token_gw = token_gw[cluster][len(token_gw[cluster])-1]
			gw_conn.Own_nodeno = no_of_gw
			return gw_conn
		}
	}
}

// func build_chord_harrary() {
// 	nodes = make(map[int]string)
// }

func main() {
	fmt.Println("Hello World from discovery_node")
	sn_cluster = append(sn_cluster, make(map[int]string))
	token_sn = append(token_sn, make(map[int]string))
	// build_chord_harrary()
	create_socket()
}
