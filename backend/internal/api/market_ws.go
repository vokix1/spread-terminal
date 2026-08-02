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
	WriteBufferSize: 1024,
	CheckOrigin: func(r *http.Request) bool {
		origin := r.Header.Get("Origin")
		return origin == "" || origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173"
	},
}

func MarketWS(router *gin.Engine, bus *market.Bus) {
	router.GET("/ws/market", func(c *gin.Context) {
		conn, err := upgrader.Upgrade(c.Writer, c.Request, nil)
		if err != nil {
			return
		}
		defer conn.Close()

		updates := bus.Subscribe(512)
		defer bus.Unsubscribe(updates)

		for {
			select {
			case <-c.Request.Context().Done():
				return
			case event, ok := <-updates:
				if !ok {
					return
				}
				_ = conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
				if err := conn.WriteJSON(event); err != nil {
					return
				}
			}
		}
	})
}
