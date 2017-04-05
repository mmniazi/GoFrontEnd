package main

import (
	"log"
	"net/http"
	"github.com/gorilla/websocket"
	"golang.org/x/net/context"
	"google.golang.org/grpc"
	pb "../proto"
	"github.com/go-redis/redis"
	"encoding/json"
)

const address = "localhost:50051"
const REDIS_KEY = "messages"

var clients = make(map[*websocket.Conn]bool) // connected clients
var broadcast = make(chan Message)           // broadcast channel
var spellCheckerClient pb.SpellCheckerClient
var grpcConn *grpc.ClientConn
var redisClient *redis.Client

// Configure the upgrader
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool {
		return true
	},
}

// Define our message object
type Message struct {
	Email    string `json:"email"`
	Username string `json:"username"`
	Message  string `json:"message"`
}

func main() {
	// Create a simple file server
	fs := http.FileServer(http.Dir("public"))
	http.Handle("/", fs)

	// Configure websocket route
	http.HandleFunc("/ws", handleConnections)

	connectToSpellChecker()
	defer grpcConn.Close()

	connectToRedis()
	defer redisClient.Close()

	// Start listening for incoming chat messages
	go handleMessages()

	//Start the server on localhost port 8000 and log any errors
	log.Println("http server started on :8000")
	err := http.ListenAndServe(":8000", nil)
	if err != nil {
		log.Fatal("ListenAndServe: ", err)
	}

}

func connectToSpellChecker() {
	// Set up a connection to the server.
	var err error
	grpcConn, err = grpc.Dial(address, grpc.WithInsecure())
	if err != nil {
		log.Fatalf("did not connect: %v", err)
	}

	spellCheckerClient = pb.NewSpellCheckerClient(grpcConn)
	log.Println(" Grpc Client started")
}

func connectToRedis() {
	redisClient = redis.NewClient(&redis.Options{
		Addr:     "localhost:6379",
		Password: "", // no password set
		DB:       0,  // use default DB
	})

	pong, err := redisClient.Ping().Result()

	if err != nil {
		log.Fatalf("failed to connect to redis: %v", err)
	} else {
		log.Println(pong + " from Redis")
	}

}

func handleConnections(w http.ResponseWriter, r *http.Request) {
	// Upgrade initial GET request to a websocket
	ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Fatal(err)
	}
	// Make sure we close the connection when the function returns
	defer ws.Close()

	// Register our new client
	clients[ws] = true

	msgs, msgsErr := redisClient.LRange(REDIS_KEY, -50, -1).Result()

	if msgsErr == nil {
		var parsedMsgs []Message = make([]Message, len(msgs))
		for i, msg := range msgs {
			var parsedMsg Message
			err := json.Unmarshal([]byte(msg), &parsedMsg)
			if err != nil {
				log.Fatal(err)
			}
			parsedMsgs[i] = parsedMsg
		}
		sendPrevMsgs(ws, parsedMsgs)
	}

	for {
		var msg Message
		// Read in a new message as JSON and map it to a Message object
		err := ws.ReadJSON(&msg)
		if err != nil {
			log.Printf("error: %v", err)
			delete(clients, ws)
			break
		}
		// Send the newly received message to the broadcast channel
		broadcast <- msg
	}
}

func handleMessages() {

	for {
		// Grab the next message from the broadcast channel
		msg := <-broadcast

		r, err := spellCheckerClient.Check(context.Background(), &pb.Request{Sentence: msg.Message})
		if err != nil {
			sendToAll(msg)
		} else {
			msg.Message = r.Sentence
			sendToAll(msg)
		}
	}
}

func sendToAll(msg Message) {

	jsonMsg, jsonErr := json.Marshal(msg)
	if jsonErr == nil {
		redisClient.RPush(REDIS_KEY, jsonMsg).Result()
	}

	// Send it out to every client that is currently connected
	for client := range clients {
		err := client.WriteJSON(msg)
		if err != nil {
			log.Printf("error: %v", err)
			client.Close()
			delete(clients, client)
		}
	}
}

func sendPrevMsgs(client *websocket.Conn, msg []Message) {

	err := client.WriteJSON(msg)
	if err != nil {
		log.Printf("error: %v", err)
		client.Close()
		delete(clients, client)
	}
}
