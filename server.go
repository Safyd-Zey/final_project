package main

import (
	"crypto/tls"
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
	banned  map[string]bool // New field to store banned users
	mutex   sync.Mutex
}

var chatRooms = make(map[string]*ChatRoom)
var chatRoomsMutex sync.Mutex

type Bot struct {
    name   string
    active bool
}

func (bot *Bot) handleMessage(conn net.Conn, message string, room *ChatRoom) {
    if !room.banned[conn.RemoteAddr().String()] && strings.ToLower(message) == "hello" {
        conn.Write([]byte(fmt.Sprintf("Bot: Hello, %s!\n", conn.RemoteAddr().String())))
    }
}


var bot *Bot

func handleConnection(conn net.Conn) {
    defer func() {
        notifyDisconnection(conn)
        conn.Close()
    }()
    notifyConnection(conn)
    reader := bufio.NewReader(conn)

    var room *ChatRoom // Store the reference to the room

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
                joinRoom(conn, arg, &room) // Pass the room reference
                if bot != nil && bot.active {
                    bot.handleMessage(conn, "hello", room)
                }
            case "/msg":
                broadcastMessage(conn, arg)
            case "/kick":
                kickUser(conn, arg)
            case "/ban":
                banUser(conn, arg)
            case "/help":
                conn.Write([]byte("Commands:\n/create <room>\n/join <room>\n/msg <message>\n/kick <User IP:Port>\n/ban <User IP:Port>\n/addbot\n"))
            case "/addbot":
                addBot()
                conn.Write([]byte("Bot added to the chat room.\n"))
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

func addBot() {
    bot = &Bot{name: "SimpleBot", active: true}
}


func banUser(conn net.Conn, userAddr string) {
	chatRoomsMutex.Lock()
	defer chatRoomsMutex.Unlock()

	for _, room := range chatRooms {
		if room.members[conn] {
			targetConn := findUserByAddr(room, userAddr)
			if targetConn != nil {
				room.mutex.Lock()
				room.banned[targetConn.RemoteAddr().String()] = true
				delete(room.members, targetConn)
				room.mutex.Unlock()

				// Send a notification to the banned user
				targetConn.Write([]byte("You have been banned from the chat room.\n"))

				room.broadcast(fmt.Sprintf("Notice: %s has been banned from the chat room.\n", userAddr))
				return
			} else {
				conn.Write([]byte(fmt.Sprintf("Error: User %s not found in the chat room.\n", userAddr)))
				return
			}
		}
	}
	conn.Write([]byte("Error: You are not in any chat room.\n"))
}

func isBanned(conn net.Conn, room *ChatRoom) bool {
	room.mutex.Lock()
	defer room.mutex.Unlock()
	return room.banned[conn.RemoteAddr().String()]
}

func kickUser(conn net.Conn, userAddr string) {
	chatRoomsMutex.Lock()
	defer chatRoomsMutex.Unlock()

	for _, room := range chatRooms {
		if room.members[conn] {
			targetConn := findUserByAddr(room, userAddr)
			if targetConn != nil {
				delete(room.members, targetConn)
				targetConn.Write([]byte("You have been kicked from the chat room.\n"))
				room.broadcast(fmt.Sprintf("Notice: %s has been kicked from the chat room.\n", userAddr))
				return
			} else {
				conn.Write([]byte(fmt.Sprintf("Error: User %s not found in the chat room.\n", userAddr)))
				return
			}
		}
	}
	conn.Write([]byte("Error: You are not in any chat room.\n"))
}

func findUserByAddr(room *ChatRoom, userAddr string) net.Conn {
	for conn := range room.members {
		if strings.Contains(conn.RemoteAddr().String(), userAddr) {
			return conn
		}
	}
	return nil
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
		banned:  make(map[string]bool), // Initialize the banned map
	}
	conn.Write([]byte(fmt.Sprintf("Created chat room \"%s\".\n", roomName)))
}

func joinRoom(conn net.Conn, roomName string, room **ChatRoom) {
    chatRoomsMutex.Lock()
    r, exists := chatRooms[roomName]
    chatRoomsMutex.Unlock()

    if !exists {
        conn.Write([]byte(fmt.Sprintf("Error: A chat room with that name does not exist.\n")))
        return
    }

    if r.banned[conn.RemoteAddr().String()] {
        conn.Write([]byte(fmt.Sprintf("Error: You are banned from chat room \"%s\".\n", roomName)))
        return
    }

    r.mutex.Lock()
    r.members[conn] = true
    r.mutex.Unlock()

    r.broadcast(fmt.Sprintf("Notice: %s joined the chat room.\n", conn.RemoteAddr()))
    conn.Write([]byte(fmt.Sprintf("Joined chat room \"%s\".\n", roomName)))

    *room = r // Update the room reference
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
	cert, err := tls.LoadX509KeyPair("server.crt", "server.key")
	if err != nil {
		log.Println("Error loading server certificate:", err)
		return
	}

	config := tls.Config{Certificates: []tls.Certificate{cert}}

	listener, err := tls.Listen(CONN_TYPE, CONN_PORT, &config)
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
