package pbftconsensus

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
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
	Participants []string
	Signature    []byte
}

type Vote struct {
	SessionId int
	Response  string
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
	PRIMARY_NODE_IP   = "150.0.0.101"
	PORT              = "8080"
	PRIMARY_NODE_CONN net.Conn
	NextGeneratorPK   = ""
	isMyTurn          chan bool
	proposalChan      chan []byte
	responseChan      chan bool
	myPk              string
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
	for {
		var err error
		var buffer []byte
		temp_buffer := make([]byte, 1024)
		var n int
		var e error
		for {
			n, e = conn.Read(temp_buffer)
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
		var message Message
		err = Deserialize(string(buffer), &message)
		return message, err
	}
}

func HelloPrimaryNode(pubKey string) {
	// Connect to the primary node
	conn, e := net.Dial("tcp", PRIMARY_NODE_IP+":"+PORT)
	PRIMARY_NODE_CONN = conn
	if e != nil {
		fmt.Println("pbft: Hello: Error connecting to primary node:", e)
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
		fmt.Println("pbft: Hello: Error sending message to primary node:", e)
		return
	}
	// Read the response from the primary node
	recvMsg, e := readMessage(conn)
	if e != nil {
		fmt.Println("pbft: Hello: Error reading message from primary node:", e)
		return
	}

	// fetch pub key of next generator from the message
	NextGeneratorPK = string(recvMsg.Data)
	// Print the response
	fmt.Println("pbft: Hello: Received message from primary node:", NextGeneratorPK)
	fmt.Println("pbft: Hello: My public key is:", pubKey)
	go recvFromPrimaryNode()
	go handleNodeConnections()

	if NextGeneratorPK == pubKey {
		go func() {
			isMyTurn <- true
		}()
	}

	// isMyTurn <- (NextGeneratorPK == myPk)

}

func Propose(proposal []byte, pubKey string) {
	conn := PRIMARY_NODE_CONN
	if conn == nil {
		fmt.Println("pbft: Propose: Connection to primary node is not established.")
		HelloPrimaryNode(pubKey)
		conn = PRIMARY_NODE_CONN
		if conn == nil {
			fmt.Println("pbft: Propose: Failed to establish connection to primary node.")
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
		fmt.Println("pbft: Propose: Error sending message to primary node:", e)
		return
	}
}

func recvFromPrimaryNode() {
	conn := PRIMARY_NODE_CONN
	if conn == nil {
		fmt.Println("pbft: recvFromPrimaryNode: Connection to primary node is not established.")
		return
	}
	// Read the response from the primary node

	for {
		recvMsg, e := readMessage(conn)
		if e != nil {
			fmt.Println("pbft: recvFromPrimaryNode: Error reading message from primary node:", e)
			return
		}
		switch recvMsg.Type {
		case "PrePrepare":
			fmt.Println("pbft: recvFromPrimaryNode: Received proposal from primary node")
			handlePrePrepare(recvMsg.Data)
		case "NextGenPK":
			// fetch pub key of next generator from the message
			NextGeneratorPK = string(recvMsg.Data)
			fmt.Println("pbft: recvFromPrimaryNode: Received message from primary node:", NextGeneratorPK)
			fmt.Println("pbft: recvFromPrimaryNode: My public key is:", myPk)
			if NextGeneratorPK == myPk {
				go func() {
					isMyTurn <- true
				}()
			}
		default:
			fmt.Println("pbft: recvFromPrimaryNode: Unknown message type:", recvMsg.Type)
		}
	}
}

func handlePrePrepare(msg []byte) {
	//deserialize the message into PrePrepare struct
	var prePrepare PrePrepare
	err := Deserialize(string(msg), &prePrepare)
	if err != nil {
		fmt.Println("pbft: handlePrePrepare: Error deserializing message:", err)
		return
	}

	// store the proposal in the current session
	currentSession.SessionId = prePrepare.SessionId
	currentSession.participants = prePrepare.Participants
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
	fmt.Println("pbft: handlePrePrepare: " + response)
	Vote := Vote{
		SessionId: prePrepare.SessionId,
		Response:  response,
		Signature: []byte("signature"), // yet to implement
	}

	fmt.Println("pbft: handlePrePrepare: ", prePrepare)

	serailizedVote, err := Serialize(Vote)
	if err != nil {
		fmt.Println("pbft: handlePrePrepare: Error serializing vote:", err)
		return
	}
	// message struct
	prepareMessage := Message{
		Type:      "Vote",
		Data:      serailizedVote,
		Timestamp: time.Now().UnixNano(),
	}
	// broadcast this message to all the nodes
	for _, participant := range prePrepare.Participants {
		connectAndSend(participant, prepareMessage)
	}

	// send the message to the primary node
	err = writeMessage(PRIMARY_NODE_CONN, prepareMessage)
	if err != nil {
		fmt.Println("pbft: handlePrePrepare: Error sending message to primary node:", err)
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
		fmt.Println("pbft: connectAndSend: Error connecting to node:", e)
		return
	}
	defer conn.Close()
	fmt.Println("pbft: connectAndSend: Connected to node:", ip)
	time.Sleep(1 * time.Second)
	e = writeMessage(conn, msg)
	fmt.Println("pbft: connectAndSend: Sent message to node:", ip)
	if e != nil {
		fmt.Println("pbft: connectAndSend: Error sending message to node:", e)
		return
	}
}

func handleNodeConnections() {
	// listen for incoming connections
	listener, err := net.Listen("tcp", ":8080")
	if err != nil {
		fmt.Println("pbft: handleNodeConnection: Error starting server:", err)
		return
	}

	defer listener.Close()

	for {
		conn, err := listener.Accept()
		if err != nil {
			fmt.Println("pbft: handleNodeConnection: Error accepting connection:", err)
			continue
		}
		go handleNodeMessages(conn)
	}
}

func handleNodeMessages(conn net.Conn) {
	defer conn.Close()
	msg, err := readMessage(conn)
	if err != nil {
		fmt.Println("pbft: handleNodeMessages: Error reading message:", err)
		return
	}
	fmt.Println("pbft: handleNodeMessages: Received message:", msg.Type)
	// handle the message based on its type
	switch msg.Type {
	case "Vote":
		fmt.Println("pbft: handleNodeMessages: Received Vote message:", string(msg.Data))
		handleVote(msg.Data)
	case "Commit":
		handleCommit()
	default:
		fmt.Println("pbft: handleNodeMessages: Unknown message type:", msg.Type)
	}

}

func handleVote(msg []byte) {
	// yet to be implemented
	var vote Vote
	err := Deserialize(string(msg), &vote)
	if err != nil {
		fmt.Println("pbft: handleVote: Error deserializing vote:", err)
		return
	}
	if currentSession.SessionId != vote.SessionId {
		fmt.Println("pbft: handleVote: Vote session id does not match current session id")
		return
	}

	// update the current session with the vote
	if vote.Response == "YES" {
		currentSession.yes++
	} else {
		currentSession.no++
	}

	fmt.Println("pbft: handleVote: Vote received. Yes:", currentSession.yes, "No:", currentSession.no, "TotalNeeded:", len(currentSession.participants))

	// if all the votes are received
	// check if the proposal is accepted or rejected
	if currentSession.yes+currentSession.no == len(currentSession.participants) {
		if float64(currentSession.yes) >= float64(len(currentSession.participants)*2)/3.0 {
			// proposal is accepted
			fmt.Println("pbft: handleVote: Proposal accepted")
			if NextGeneratorPK == myPk {
				responseChan <- true
			}
			// send the commit message to all the nodes
			sendCommit()
		} else {
			if NextGeneratorPK == myPk {
				responseChan <- false
			}
			fmt.Println("pbft: handleVote: Proposal rejected")
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
		fmt.Println("pbft: sendCommit: Error sending message to primary node:", err)
		return
	}
	fmt.Println("pbft: sendCommit: Commit message sent to all nodes")
}

func handleCommit() {
	currentSession.commit++
	if float64(currentSession.commit) >= float64(len(currentSession.participants)*2)/float64(3) {
		//add proposal to the proposal list
		ProposalList = append(ProposalList, currentSession.proposal)

		currentSession.commit = 0
	}
}

func RunPbftConsensusModule(pk string, turn *chan bool, proposalChannel *chan []byte, responseChannel *chan bool) {
	myPk = pk
	isMyTurn = *turn
	proposalChan = *proposalChannel
	responseChan = *responseChannel
	ProposalList = make([][]byte, 0)
	// connect to the primary node
	HelloPrimaryNode(pk)

	fmt.Println("pbft: Pbft consensus module started")
	// wait for the proposal
	for {
		proposal := <-proposalChan
		// check if it is my turn
		if NextGeneratorPK != myPk {
			responseChan <- false
			continue
		}
		Propose(proposal, myPk)
	}
}
