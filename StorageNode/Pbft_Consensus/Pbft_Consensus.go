package pbftconsensus

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
	// ckm "LSDI_Gateway/Crypt_key_Manager"
)

type Message struct {
	Type      string
	Data      []byte
	Timestamp int64
}

type PrePrepare struct {
	SessionId    int
	Proposal     []byte
	participants []string
	Signature    []byte
}

type Vote struct {
	SessionId int
	response  string
	Signature []byte
}

type CurrentSession struct {
	SessionId    int
	proposal     []byte
	yes          int
	no           int
	commit       int
	participants []string
}

var (
	PRIMARY_NODE_IP   = "150.0.0.2"
	PORT              = "8080"
	PRIMARY_NODE_CONN net.Conn
	NextGeneratorPK   = ""
	isMyTurn chan bool
	currentSession    CurrentSession
	ProposalList      [][]byte
)

func Serialize(p interface{}) ([]byte, error) {
	return json.Marshal(p)
}

func Deserialize(data string, p any) error {
	return json.Unmarshal(json.RawMessage(data), p)
}

func writeMessage(conn net.Conn, msg Message) error {
	// Serialize the message
	data, err := Serialize(msg)
	if err != nil {
		return err
	}
	// Send the actual message
	_, err = conn.Write(data)
	if err != nil {
		return err
	}

	return nil
}

func readMessage(conn net.Conn) (Message, error) {
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
		return Message{}, err
	}
	var message Message
	err = Deserialize(string(buffer), message)
	return message, err
}

func HelloPrimaryNode(pubKey string) {
	// Connect to the primary node
	conn, e := net.Dial("tcp", PRIMARY_NODE_IP+":"+PORT)
	PRIMARY_NODE_CONN = conn
	if e != nil {
		fmt.Println("Error connecting to primary node:", e)
		return
	}

	// Send a message to the primary node
	helloMessage := Message{
		Type:      "Hello",
		Data:      []byte("pubKey: " + pubKey),
		Timestamp: time.Now().UnixNano(),
	}
	e = writeMessage(conn, helloMessage)
	if e != nil {
		fmt.Println("Error sending message to primary node:", e)
		return
	}
	// Read the response from the primary node
	recvMsg, e := readMessage(conn)
	if e != nil {
		fmt.Println("Error reading message from primary node:", e)
		return
	}

	// fetch pub key of next generator from the message
	NextGeneratorPK = strings.Split(string(recvMsg.Data), " ")[1]
	// Print the response
	fmt.Println("Received message from primary node:", NextGeneratorPK)
	
	go recvFromPrimaryNode()
	go handleNodeConnections()
}

func Propose(proposal []byte, pubKey string) {
	conn := PRIMARY_NODE_CONN
	if conn == nil {
		fmt.Println("Connection to primary node is not established.")
		HelloPrimaryNode(pubKey)
		conn = PRIMARY_NODE_CONN
		if conn == nil {
			fmt.Println("Failed to establish connection to primary node.")
			return
		}
	}
	// Send a message to the primary node
	proposeMessage := Message{
		Type:      "Propose",
		Data:      proposal,
		Timestamp: time.Now().UnixNano(),
	}
	e := writeMessage(conn, proposeMessage)
	if e != nil {
		fmt.Println("Error sending message to primary node:", e)
		return
	}
}

func recvFromPrimaryNode() {
	conn := PRIMARY_NODE_CONN
	if conn == nil {
		fmt.Println("Connection to primary node is not established.")
		return
	}
	// Read the response from the primary node

	for {
		recvMsg, e := readMessage(conn)
		if e != nil {
			fmt.Println("Error reading message from primary node:", e)
			return
		}
		switch recvMsg.Type {
		case "PrePrepare":
			fmt.Println("Received proposal from primary node")
			handlePrePrepare(recvMsg.Data)
		case "NextGenPK":
			// fetch pub key of next generator from the message
			NextGeneratorPK = strings.Split(string(recvMsg.Data), " ")[1]
			fmt.Println("Received message from primary node:", NextGeneratorPK)
		default:
			fmt.Println("Unknown message type:", recvMsg.Type)
		}
	}
}

func handlePrePrepare(msg []byte) {
	//deserialize the message into PrePrepare struct
	var prePrepare PrePrepare
	err := Deserialize(string(msg), &prePrepare)
	if err != nil {
		fmt.Println("Error deserializing message:", err)
		return
	}

	// store the proposal in the current session
	currentSession.SessionId = prePrepare.SessionId
	currentSession.participants = prePrepare.participants
	currentSession.yes = 0
	currentSession.no = 0
	currentSession.commit = 0
	currentSession.proposal = prePrepare.Proposal

	response := ""
	if validProposal(prePrepare.Proposal) {
		response = "YES"
	} else {
		response = "NO"
	}
	fmt.Println(response)
	Vote := Vote{
		SessionId: prePrepare.SessionId,
		response:  response,
		Signature: []byte("signature"), // yet to implement
	}

	serailizedVote, err := Serialize(Vote)
	if err != nil {
		fmt.Println("Error serializing vote:", err)
		return
	}
	// message struct
	prepareMessage := Message{
		Type:      "Vote",
		Data:      serailizedVote,
		Timestamp: time.Now().UnixNano(),
	}
	// broadcast this message to all the nodes
	for _, participant := range prePrepare.participants {
		connectAndSend(participant, prepareMessage)
	}

	// send the message to the primary node
	err = writeMessage(PRIMARY_NODE_CONN, prepareMessage)
	if err != nil {
		fmt.Println("Error sending message to primary node:", err)
		return
	}
}

func validProposal(proposal []byte) bool {

	// yet to implement
	_ = proposal
	return true
}

func connectAndSend(ip string, msg Message) {
	conn, e := net.Dial("tcp", ip)
	if e != nil {
		fmt.Println("Error connecting to node:", e)
		return
	}
	defer conn.Close()

	e = writeMessage(conn, msg)
	if e != nil {
		fmt.Println("Error sending message to node:", e)
		return
	}
}

func handleNodeConnections() {
	// listen for incoming connections
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("Error starting server:", err)
		return
	}

	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("Error accepting connection:", err)
			continue
		}
		go handleNodeMessages(conn)
	}
}

func handleNodeMessages(conn net.Conn) {
	defer conn.Close()
	msg, err := readMessage(conn)
	if err != nil {
		fmt.Println("Error reading message:", err)
		return
	}
	// handle the message based on its type
	switch msg.Type {
	case "Vote":
		handleVote(msg.Data)
	case "Commit": 
		handleCommit()
	default:
		fmt.Println("Unknown message type:", msg.Type)
	}

}

func handleVote(msg []byte) {
	// yet to be implemented
	var vote Vote
	err := Deserialize(string(msg), &vote)
	if err != nil {
		fmt.Println("Error deserializing vote:", err)
		return
	}
	if currentSession.SessionId != vote.SessionId {
		fmt.Println("Vote session id does not match current session id")
		return
	}
	// update the current session with the vote
	if vote.response == "YES" {
		currentSession.yes++
	} else {
		currentSession.no++
	}

	// if all the votes are received
	// check if the proposal is accepted or rejected
	if currentSession.yes+currentSession.no == len(currentSession.participants) {
		if currentSession.yes >= len(currentSession.participants)*2/3 {
			// proposal is accepted
			fmt.Println("Proposal accepted")
			// send the commit message to all the nodes
			sendCommit()
		} else {
			fmt.Println("Proposal rejected")
		}
	}
}

func sendCommit() {
	// yet to be implemented
	commitMessage := Message{
		Type:      "Commit",
		Data:      []byte("Proposal accepted"),
		Timestamp: time.Now().UnixNano(),
	}
	// broadcast this message to all the nodes
	for _, participant := range currentSession.participants {
		connectAndSend(participant, commitMessage)
	}

	// send the message to the primary node
	err := writeMessage(PRIMARY_NODE_CONN, commitMessage)
	if err != nil {
		fmt.Println("Error sending message to primary node:", err)
		return
	}
	fmt.Println("Commit message sent to all nodes")
}

func handleCommit() {
	currentSession.commit++
	if currentSession.commit >= len(currentSession.participants)*2/3 {
		//add proposal to the proposal list
		ProposalList = append(ProposalList, currentSession.proposal)
		currentSession.commit = 0
	}
}
