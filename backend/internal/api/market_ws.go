package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/vokix1/spread-terminal/backend/internal/market"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 16 * 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" ||
			origin == "http://localhost:5173" ||
			origin == "http://127.0.0.1:5173" ||
			origin == "http://localhost:3000"
	},
}

// MarketWS registers the /ws/market endpoint.
// The client receives a stream of Opportunity JSON objects.
func MarketWS(router *gin.Engine, bus *market.Bus) {
	router.GET("/ws/market", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		updates := bus.Subscribe(1024)
		defer bus.Unsubscribe(updates)

		// Ping loop to detect dead connections.
		go func() {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-c.Request.Context().Done():
					return
				case <-ticker.C:
					_ = conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
					if err := conn.WriteMessage(websocket.PingMessage, nil); err != nil {
						return
					}
				}
			}
		}()

		for {
			select {
			case <-c.Request.Context().Done():
				return
			case opp, ok := <-updates:
				if !ok {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteJSON(opp); err != nil {
					return
				}
			}
		}
	})
}
