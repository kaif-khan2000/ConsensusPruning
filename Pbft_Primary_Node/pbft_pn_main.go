package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"strings"
	"sync"
	"time"
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

type CurrentSession struct {
	SessionId    int
	proposal     []byte
	yes          int
	no           int
	commit       int
	participants []string
}

type Vote struct {
	SessionId int
	response  string
	Signature []byte
}

var (
	participants         = make(map[string]net.Conn)
	participantArray     = []string{}
	participantsMutex    sync.Mutex
	NextGeneratorPK      = ""
	nextGeneratorIndex   = -1
	NextGeneratorPKMutex sync.Mutex
	currentSession       CurrentSession
	currentSessionMutex  sync.Mutex
	ProposalList         = [][]byte{}
	ProposalListMutex    sync.Mutex
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

func handleHello(conn net.Conn, msg Message) {
	// get the public key from the message
	publicAddressOfNode := strings.Split(string(msg.Data), " ")[1]

	// store it in the map
	participantsMutex.Lock()
	participants[publicAddressOfNode] = conn
	participantsMutex.Unlock()

	NextGeneratorPKMutex.Lock()
	if NextGeneratorPK == "" {
		// set the next generator public key
		NextGeneratorPK = publicAddressOfNode
		// set the next generator index
		nextGeneratorIndex = 0
	}
	// send a message back to the node
	response := Message{
		Type:      "NextGenPk",
		Data:      []byte(NextGeneratorPK),
		Timestamp: msg.Timestamp,
	}
	NextGeneratorPKMutex.Unlock()

	err := writeMessage(conn, response)
	if err != nil {
		fmt.Println("Error sending message to node:", err)
		return
	}
	fmt.Println("Sent NextGenPk message to:", publicAddressOfNode)
}

func handlePropose(msg Message) {
	// make a preprepare message and send it to all the participants
	currentSessionMutex.Lock()
	currentSession.SessionId++
	currentSession.proposal = msg.Data
	currentSession.yes = 0
	currentSession.no = 0
	currentSession.commit = 0
	currentSessionMutex.Unlock()

	participantsMutex.Lock()
	participantList := make([]string, 0)
	for _, participant := range participants {
		// get the ip and port of the participant conn
		ip := participant.RemoteAddr().(*net.TCPAddr).IP.String()
		port := participant.RemoteAddr().(*net.TCPAddr).Port
		participantList = append(participantList, fmt.Sprintf("%s:%d", ip, port))
	}
	participantsMutex.Unlock()

	currentSessionMutex.Lock()
	prePrepare := PrePrepare{
		SessionId:    currentSession.SessionId,
		Proposal:     currentSession.proposal,
		participants: participantList,
		Signature:    []byte("signature"), // yet to implement
	}
	currentSessionMutex.Unlock()

	prePrepareBytes, err := Serialize(prePrepare)
	if err != nil {
		fmt.Println("Error serializing PrePrepare message:", err)
		return
	}

	prePrepareMessage := Message{
		Type:      "PrePrepare",
		Data:      prePrepareBytes,
		Timestamp: time.Now().UnixNano(),
	}

	participantsMutex.Lock()
	for _, participant := range participants {
		err := writeMessage(participant, prePrepareMessage)
		if err != nil {
			fmt.Println("Error sending PrePrepare message to participant:", err)
			continue
		}
		fmt.Println("Sent PrePrepare message to participant")
	}
	participantsMutex.Unlock()
}

func handleVote(msg []byte) {
	// yet to be implemented
	var vote Vote
	err := Deserialize(string(msg), &vote)
	if err != nil {
		fmt.Println("Error deserializing vote:", err)
		return
	}
	currentSessionMutex.Lock()
	if currentSession.SessionId != vote.SessionId {
		fmt.Println("Vote session id does not match current session id")
		currentSessionMutex.Unlock()
		return
	}
	// update the current session with the vote
	if vote.response == "YES" {
		currentSession.yes++
	} else {
		currentSession.no++
	}
	currentSessionMutex.Unlock()

	// // if all the votes are received
	// // check if the proposal is accepted or rejected
	// if currentSession.yes+currentSession.no == len(currentSession.participants) {
	// 	if currentSession.yes >= len(currentSession.participants)*2/3 {
	// 		// proposal is accepted
	// 		fmt.Println("Proposal accepted")
	// 		// send the commit message to all the nodes
	// 		sendCommit()
	// 	} else {
	// 		fmt.Println("Proposal rejected")
	// 	}
	// }
}

func handleCommit() {
	currentSessionMutex.Lock()
	currentSession.commit++
	if currentSession.commit >= len(currentSession.participants) {
		//add proposal to the proposal list
		ProposalList = append(ProposalList, currentSession.proposal)
		currentSession.commit = 0
	}
	currentSessionMutex.Unlock()

	// now create a new nextgenpk
	NextGeneratorPKMutex.Lock()
	nextGeneratorIndex = (nextGeneratorIndex + 1) % len(participantArray)
	NextGeneratorPK = participantArray[nextGeneratorIndex]
	NextGeneratorPKMutex.Unlock()
	// send the next generator pk to all the participants
	nextGenMsg := Message{
		Type:      "NextGenPk",
		Data:      []byte(NextGeneratorPK),
		Timestamp: time.Now().UnixNano(),
	}

	participantsMutex.Lock()
	for _, participant := range participants {
		err := writeMessage(participant, nextGenMsg)
		if err != nil {
			fmt.Println("Error sending NextGenPk message to participant:", err)
			continue
		}
		fmt.Println("Sent NextGenPk message to participant")
	}
	participantsMutex.Unlock()

}

func handleConnection(conn net.Conn) {
	for {
		msg, err := readMessage(conn)
		if err != nil {
			fmt.Println("Error reading message:", err)
			return
		}

		switch msg.Type {
		case "Hello":
			fmt.Println("Received Hello message:", string(msg.Data))
			// Handle Hello message
			handleHello(conn, msg)
		case "Propose":
			fmt.Println("Received Propose message:", string(msg.Data))
			// Handle Data message
			handlePropose(msg)
		case "Vote":
			fmt.Println("Received Vote message:", string(msg.Data))
			// Handle Goodbye message
			handleVote(msg.Data)
		case "Commit":
			fmt.Println("Received Commit message:", string(msg.Data))
			// Handle Goodbye message
			handleCommit()
		default:
			fmt.Println("Unknown message type:", msg.Type)
		}
	}
}

func nodeListener() {
	// Listen for incoming connections
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
		go handleConnection(conn)
	}
}

func main() {
	fmt.Println("Starting PBFT Primary Node...")
	nodeListener()
}
