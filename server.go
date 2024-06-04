package main

import (
	"bufio"
	"fmt"
	"log"
	"net"
	"os"
	"strings"
	"sync"
)

const (
	CONN_PORT = ":3334"
	CONN_TYPE = "tcp"
)

type ChatRoom struct {
	name    string
	members map[net.Conn]bool
	mutex   sync.Mutex
}

var chatRooms = make(map[string]*ChatRoom)
var chatRoomsMutex sync.Mutex

func handleConnection(conn net.Conn) {
	defer func() {
		notifyDisconnection(conn)
		conn.Close()
	}()
	notifyConnection(conn)
	reader := bufio.NewReader(conn)

	for {
		message, err := reader.ReadString('\n')
		if err != nil {
			log.Println("Error reading from client:", err)
			return
		}

		message = strings.TrimSpace(message)
		if strings.HasPrefix(message, "/") {
			commands := strings.SplitN(message, " ", 2)
			cmd := commands[0]
			arg := ""
			if len(commands) > 1 {
				arg = commands[1]
			}

			switch cmd {
			case "/create":
				createRoom(conn, arg)
			case "/join":
				joinRoom(conn, arg)
			case "/msg":
				broadcastMessage(conn, arg)
			case "/help":
				conn.Write([]byte("Commands:\n/create <room>\n/join <room>\n/msg <message>\n"))
			default:
				conn.Write([]byte("Unknown command. Type /help for a list of commands.\n"))
			}
		} else if message == "User is typing..." {
			notifyTyping(conn)
		} else {
			conn.Write([]byte("Unknown command. Type /help for a list of commands.\n"))
		}
	}
}

func createRoom(conn net.Conn, roomName string) {
	chatRoomsMutex.Lock()
	defer chatRoomsMutex.Unlock()

	if _, exists := chatRooms[roomName]; exists {
		conn.Write([]byte("Error: A chat room with that name already exists.\n"))
		return
	}

	chatRooms[roomName] = &ChatRoom{
		name:    roomName,
		members: make(map[net.Conn]bool),
	}
	conn.Write([]byte(fmt.Sprintf("Created chat room \"%s\".\n", roomName)))
}

func joinRoom(conn net.Conn, roomName string) {
	chatRoomsMutex.Lock()
	room, exists := chatRooms[roomName]
	chatRoomsMutex.Unlock()

	if !exists {
		conn.Write([]byte(fmt.Sprintf("Error: A chat room with that name does not exist.\n")))
		return
	}

	room.mutex.Lock()
	room.members[conn] = true
	room.mutex.Unlock()

	room.broadcast(fmt.Sprintf("Notice: %s joined the chat room.\n", conn.RemoteAddr()))
	conn.Write([]byte(fmt.Sprintf("Joined chat room \"%s\".\n", roomName)))
}

func (room *ChatRoom) broadcast(message string) {
	room.mutex.Lock()
	defer room.mutex.Unlock()
	for member := range room.members {
		member.Write([]byte(message))
	}
}

func broadcastMessage(conn net.Conn, message string) {
	chatRoomsMutex.Lock()
	defer chatRoomsMutex.Unlock()

	for _, room := range chatRooms {
		if room.members[conn] {
			room.broadcast(fmt.Sprintf("%s: %s\n", conn.RemoteAddr(), message))
			return
		}
	}
	conn.Write([]byte("Error: You are not in any chat room.\n"))
}

func notifyConnection(conn net.Conn) {
	broadcastGlobal(fmt.Sprintf("Notice: %s is online.\n", conn.RemoteAddr()))
}

func notifyDisconnection(conn net.Conn) {
	broadcastGlobal(fmt.Sprintf("Notice: %s went offline.\n", conn.RemoteAddr()))
}

func notifyTyping(conn net.Conn) {
	broadcastGlobal(fmt.Sprintf("Notice: %s is typing...\n", conn.RemoteAddr()))
}

func broadcastGlobal(message string) {
	chatRoomsMutex.Lock()
	defer chatRoomsMutex.Unlock()

	for _, room := range chatRooms {
		room.broadcast(message)
	}
}

func main() {
	listener, err := net.Listen(CONN_TYPE, CONN_PORT)
	if err != nil {
		log.Println("Error:", err)
		os.Exit(1)
	}
	defer listener.Close()
	log.Println("Listening on " + CONN_PORT)

	for {
		conn, err := listener.Accept()
		if err != nil {
			log.Println("Error:", err)
			continue
		}
		go handleConnection(conn)
	}
}
