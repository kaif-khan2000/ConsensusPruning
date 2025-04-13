package main

import (
	ckm "LSDI_Gateway/Crypt_key_Manager"
	dh "LSDI_Gateway/Data_Handler"
	p2p "LSDI_Gateway/P2P_Manager"
	"io/ioutil"
	"net/http"
	"strconv"

	"crypto/ecdsa"
	"encoding/json"
	"fmt"
	"math/rand"
	"time"

	// for dag image
	"bufio"
	"encoding/hex"
	"os"
	"os/exec"
	"sort"

	"github.com/dominikbraun/graph"
	"github.com/dominikbraun/graph/draw"

	"github.com/gin-gonic/gin"
)

// global variables
var (
	// Chunk array
	dag          dh.DAG
	gwPrivateKey *ecdsa.PrivateKey
	gwPublicKey  ecdsa.PublicKey
	TxRate       float64
)

type SideChainBlock struct {
	Hash       [32]byte
	PrevHash   [32]byte
	From       [65]byte
	Timestamp  int64
	Nonce      int
	Difficulty int
	Data       string
	Signature  []byte
}

// get the image of the dag
func getDagImage(c *gin.Context) {
	// sort the dag graph by timestamp
	dagGraph := dag.Graph
	keys := make([]string, 0, len(dagGraph))
	for k := range dagGraph {
		keys = append(keys, k)
	}

	dag.Mux.Lock()
	fmt.Println("dag acquired by getDagImage")
	// sort the keys
	sort.SliceStable(keys, func(i, j int) bool {
		return dagGraph[keys[i]].Tx.Timestamp < dagGraph[keys[j]].Tx.Timestamp
	})

	keyMap := make(map[string]string)
	strHash := func(c string) string {
		return c
	}
	// create the dag image
	g := graph.New(strHash, graph.Directed())
	for i := range keys {
		name := strconv.Itoa(dagGraph[keys[i]].Weight) + ":" + strconv.Itoa(dagGraph[keys[i]].Vertex_no) + ":" + strconv.Itoa(dagGraph[keys[i]].Gateway_no)
		// if weight == 0 or 1 and gateway_no == 0 and vertex_no == 0 then print keys[i]
		if dagGraph[keys[i]].Gateway_no == 0 && dagGraph[keys[i]].Vertex_no == 0 {
			fmt.Println(": ", keys[i])
		}
		_ = g.AddVertex(name)
		keyMap[keys[i]] = name
	}

	// add the edges
	for key, value := range keyMap {
		left := dagGraph[key].Tx.LeftTip
		right := dagGraph[key].Tx.RightTip
		leftTip := hex.EncodeToString(left[:])
		rightTip := hex.EncodeToString(right[:])
		v1 := keyMap[leftTip]
		v2 := keyMap[rightTip]
		err := g.AddEdge(value, v1, graph.EdgeWeight(1))
		if err != nil {
			if err.Error() != "target vertex 0: vertex not found" {
				fmt.Println("Error in adding the edge: ", err.Error())
			}
		}
		if keyMap[leftTip] != keyMap[rightTip] {
			err = g.AddEdge(value, v2, graph.EdgeWeight(2))
			if err != nil {
				if err.Error() != "target vertex 0: vertex not found" {
					fmt.Println("Error in adding the edge: ", err.Error())
				}
			}
		}
	}
	dag.Mux.Unlock()
	file, err := os.Create("image.gv")
	_ = draw.DOT(g, file)
	fmt.Println("Graph created,", file, err)

	_, err = exec.Command("dot", "-Tpng", "image.gv", "-o", "pics/image.png").Output()
	if err != nil {
		fmt.Println("Error in creating the image: ", err.Error())
	}
	img, err := os.Open("pics/image.png")
	if err != nil {
		fmt.Println("Error in opening the image: ", err.Error())
	}
	defer img.Close()
	imgInfo, _ := img.Stat()
	var size int64 = imgInfo.Size()
	imgBytes := make([]byte, size)
	// read image into bytes
	buffer := bufio.NewReader(img)
	_, err = buffer.Read(imgBytes)
	if err != nil {
		fmt.Println("Error in reading the image: ", err.Error())
	}

	// conert to base64
	// imgBase64 := base64.StdEncoding.EncodeToString(imgBytes)
	// render the image
	c.File("index.html")
}

func simulateTransactions() {
	fmt.Println("Running the simulation")
	time1 := time.Now()
	for {
		fmt.Println("new transaction creation")
		rand.Seed(time.Now().UnixNano())
		tempData := rand.Float64()
		tempHash := ckm.GenerateHashOfString(strconv.FormatFloat(tempData, 'f', -1, 64))
		transaction := dh.CreateTx(gwPrivateKey, &dag, tempHash)
		vertex := dh.AddToDAG(transaction, &dag, gwPrivateKey)
		p2p.BroadcastVtx(vertex)
		time2 := time.Now()
		fmt.Println("transactionRate: ", time2.Sub(time1), " seconds")
		time1 = time2
	}
}

func getTx(c *gin.Context) {
	txHash := c.Param("tx_hash")
	dag.Mux.Lock()
	tx, ok := dag.Graph[txHash]
	if !ok {
		dag.Mux.Unlock()
		c.JSON(http.StatusNotFound, gin.H{
			"error": "transaction not found",
		})
		return
	}
	time1 := tx.Tx.Timestamp
	dag.Mux.Unlock()
	fmt.Println(": time1: ", time1)
	c.IndentedJSON(http.StatusOK, tx)
}

func gwHTTPServer() {
	// gin server
	router := gin.Default()
	router.Static("/pics", "./pics")
	router.GET("/getDagImage", getDagImage)
	router.GET("/getTx/:tx_hash", getTx)
	router.Run(":4000")
}

func getSNInfoFromConfig() (string, string) {
	// read the config file
	file, err := ioutil.ReadFile("config.json")
	if err != nil {
		fmt.Println("Error in reading the config file: ", err.Error())
	}
	var config map[string]string
	err = json.Unmarshal(file, &config)
	if err != nil {
		fmt.Println("Error in unmarshalling the config file: ", err.Error())
	}
	snIP := config["seed_sn_ip"]
	snPort := config["seed_sn_port"]

	//convert snPort to int
	// snPortInt, err := strconv.Atoi(snPort)

	return snIP, snPort
}

type AdamPointState struct {
	AdamPoint [32]byte
	PruneList []int
}

func getPruningListFromSN(snIP, snPort string) AdamPointState {
	// get the pruning list from the sn using the http request
	url := "http://" + snIP + ":" + snPort + "/getLatestPruneList"
	resp, err := http.Get(url)
	if err != nil {
		fmt.Println("Error in getting the pruning list from the sn: ", err.Error())
	}
	defer resp.Body.Close()
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		fmt.Println("Error in reading the response body: ", err.Error())
	}

	fmt.Println("pruning list: ", string(body))

	var pruningList AdamPointState
	err = json.Unmarshal(body, &pruningList)

	if err != nil {
		fmt.Println("Error in unmarshalling the pruning list: ", err.Error())
	}

	// fmt.Println("pruning list: ", pruningList)

	// from the json get the data
	// data := pruningList["data"]

	return pruningList
}

func periodicallyGetPruningListFromSN() {
	snIP, snPort := getSNInfoFromConfig()
	fmt.Println("pruning list: sn ip: ", snIP)
	fmt.Println("pruning list: sn port: ", snPort)
	time.Sleep(10 * time.Minute)
	for {
		time.Sleep(3 * time.Minute)
		// get the pruning list from the sn using the http request
		pruningList := getPruningListFromSN(snIP, snPort)
		// fmt.Println("pruning list: ", pruningList)
		// pruningList := getPruningListFromSN(snIP, snPort)
		fmt.Println("pruning list: ", pruningList)

		adamPoint := hex.EncodeToString(pruningList.AdamPoint[:])

		fmt.Println("adamPoint: ", adamPoint)
		referenceList := []string{}
		// var referenceStrings []string
		for _, referencePath := range pruningList.PruneList {
			// convert the int to bitstring
			bitString := fmt.Sprintf("%b", referencePath)

			// append extra zeroes at the start of the bitstring
			for len(bitString) < 32 {
				bitString = "0" + bitString
			}

			fmt.Println(referencePath, " : ", bitString)
			// now fetch the first 4 bits of the bitstring and convert it to int
			referenceSize := bitString[:4]
			fmt.Println("referenceSize: ", referenceSize)
			// convert the referenceSize to int
			referenceSizeInt := 0
			for i := 0; i < 4; i++ {
				referenceSizeInt = referenceSizeInt*2 + int(referenceSize[i]-'0')
				fmt.Println("referenceSizeInt: ", referenceSizeInt)
			}
			fmt.Println("referenceSizeInt: ", referenceSizeInt)

			// now get that many bits from the bitstring from the last as the reference
			reference := bitString[32-referenceSizeInt:]
			fmt.Println("reference: ", reference)

			// append the reference to the referenceList
			referenceList = append(referenceList, reference)

		}
		fmt.Println("referenceList: ", referenceList)
		dh.AdamPointPrune(adamPoint, referenceList, &dag)
	}
}

func demo() {
	// create keys for the gateway
	gwPrivateKey, gwPublicKey = ckm.GenerateKeyPair()
	serPubKey, _ := json.Marshal(gwPublicKey)
	fmt.Println("gwPublicKey:> ", gwPublicKey)
	fmt.Println("", gwPublicKey.X, gwPublicKey.Y)
	fmt.Println("", string(serPubKey))
	// sessionExpiry = 3

	// avgRecRate = 100
	// generate session id
	// createSession()

	fmt.Println("successfully generated keys for the gateway")
	fmt.Println("\ngateway private key: ", gwPrivateKey)
	fmt.Println("\ngateway public key: ", gwPublicKey)
	// fmt.Println("\nsession id: ", session.Id)

	// Initialize the DAG
	dh.InitDag(&dag)

	go gwHTTPServer()

	// connect to the css
	// err := connectToCss()
	// if err != nil {
	// 	fmt.Println(err)
	// 	return
	// }
	// create genesis transaction
	genesisTransaction := dh.CreateGenesisTx()
	fmt.Println("\nsuccessfully created genesis transaction")
	//add the genesis transaction to the dag
	dh.AddToDAG(genesisTransaction, &dag, gwPrivateKey)
	fmt.Println("\nsuccessfully added genesis transaction to the dag")

	time.Sleep(30 * time.Second)
	// now make connections with other gateways and start listening for connections
	p2ptest := make(chan bool)
	go p2p.Gateway(&dag, gwPrivateKey, p2ptest)
	// now start listening for connections
	// p2pStartListening <- true

	go periodicallyGetPruningListFromSN()

	test := <-p2ptest
	fmt.Println(test)
	//check network connectivity
	if true {
		fmt.Println("p2p setup successful")
		simulateTransactions()
	} else {
		fmt.Println("Sensor is not running")
	}
	// for {

	// }
}

func main() {

	// snIp, snPort := getSNInfoFromConfig()
	// fmt.Println("sn ip: ", snIp)
	// fmt.Println("sn port: ", snPort)
	time.Sleep(1 * time.Minute)
	demo()
}
