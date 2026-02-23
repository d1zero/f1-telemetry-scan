package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"
	"sync"

	"github.com/gorilla/websocket"
)

// Константы и настройки
var (
	UDP_HOST              = getEnv("UDP_HOST", "0.0.0.0")
	UDP_PORT              = getEnv("UDP_PORT", "20777")
	SAMPLE_EVERY_N_FRAMES = getEnvInt("SAMPLE_EVERY_N_FRAMES", 2) // Для плавности на фронте лучше 2
)

const (
	PID_CAR_TELEMETRY = 6
	HDR_SIZE          = 29
	TEL_SIZE          = 60
)

// --- Структуры данных с JSON тегами ---

type PacketHeader struct {
	PacketFormat     uint16
	GameYear         uint8
	GameMajorVersion uint8
	GameMinorVersion uint8
	PacketVersion    uint8
	PacketId         uint8
	SessionUID       uint64
	SessionTime      float32
	FrameIdentifier  uint32
	OverallFrameId   uint32
	PlayerCarIndex   uint8
	SecondaryPlayer  int8
}

type CarTelemetryData struct {
	Speed             uint16     `json:"speed"`
	Throttle          float32    `json:"throttle"`
	Steer             float32    `json:"steer"`
	Brake             float32    `json:"brake"`
	Clutch            uint8      `json:"clutch"`
	Gear              int8       `json:"gear"`
	EngineRPM         uint16     `json:"rpm"`
	DRS               uint8      `json:"drs"`
	RevLightsPercent  uint8      `json:"rev_lights"`
	RevLightsBitValue uint16     `json:"rev_lights_bit"`
	BrakesTemp        [4]uint16  `json:"brakes_temp"`
	TyresSurfaceTemp  [4]uint8   `json:"tyres_surface_temp"`
	TyresInnerTemp    [4]uint8   `json:"tyres_inner_temp"`
	EngineTemp        uint16     `json:"engine_temp"`
	TyresPressure     [4]float32 `json:"tyres_pressure"`
	SurfaceType       [4]uint8   `json:"surface_type"`
}

// --- WebSocket Hub для управления подключениями ---

type Hub struct {
	clients   map[*websocket.Conn]bool
	broadcast chan CarTelemetryData
	mu        sync.Mutex
}

func newHub() *Hub {
	return &Hub{
		clients:   make(map[*websocket.Conn]bool),
		broadcast: make(chan CarTelemetryData),
	}
}

func (h *Hub) run() {
	for {
		data := <-h.broadcast
		h.mu.Lock()
		for client := range h.clients {
			err := client.WriteJSON(data)
			if err != nil {
				log.Printf("error: %v", err)
				client.Close()
				delete(h.clients, client)
			}
		}
		h.mu.Unlock()
	}
}

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

// --- Main ---

func main() {
	hub := newHub()
	go hub.run()

	// 1. Маршрут для WebSocket
	http.HandleFunc("/telemetry", func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			log.Print("upgrade:", err)
			return
		}
		hub.mu.Lock()
		hub.clients[conn] = true
		hub.mu.Unlock()
		log.Println("New client connected")
	})

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
	})

	// 2. Запуск веб-сервера в фоне
	go func() {
		fmt.Println("[WS] Server started on :8080/telemetry")
		if err := http.ListenAndServe("0.0.0.0:8080", nil); err != nil {
			log.Fatal("ListenAndServe:", err)
		}
	}()

	// 3. Работа с UDP (основной поток)
	addr := fmt.Sprintf("%s:%s", UDP_HOST, UDP_PORT)
	udpConn, err := net.ListenPacket("udp", addr)
	if err != nil {
		log.Fatal(err)
	}
	defer udpConn.Close()

	fmt.Printf("[UDP] Listening on %s\n", addr)

	buf := make([]byte, 4096)
	var lastFrame uint32
	var firstPacket = true

	for {
		n, _, err := udpConn.ReadFrom(buf)
		if err != nil {
			continue
		}

		if n < HDR_SIZE {
			continue
		}

		var hdr PacketHeader
		binary.Read(bytes.NewReader(buf[:HDR_SIZE]), binary.LittleEndian, &hdr)

		if hdr.PacketId != PID_CAR_TELEMETRY {
			continue
		}

		// Сэмплирование
		if !firstPacket && hdr.OverallFrameId == lastFrame {
			continue
		}
		if hdr.OverallFrameId%uint32(SAMPLE_EVERY_N_FRAMES) != 0 {
			continue
		}
		lastFrame = hdr.OverallFrameId
		firstPacket = false

		offset := HDR_SIZE + (int(hdr.PlayerCarIndex) * TEL_SIZE)
		if n < offset+TEL_SIZE {
			continue
		}

		var tel CarTelemetryData
		binary.Read(bytes.NewReader(buf[offset:offset+TEL_SIZE]), binary.LittleEndian, &tel)

		// Отправляем в канал для рассылки всем вебсокет-клиентам
		hub.broadcast <- tel
	}
}

// Хелперы
func getEnv(key, fallback string) string {
	if value, ok := os.LookupEnv(key); ok {
		return value
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if value, ok := os.LookupEnv(key); ok {
		i, _ := strconv.Atoi(value)
		return i
	}
	return fallback
}
