package api


import (

	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/gorilla/websocket"

	"github.com/vokix1/spread-terminal/backend/internal/market"

)



var upgrader =
websocket.Upgrader{

	CheckOrigin:
	func(r *http.Request) bool {
		return true
	},

}



func MarketWS(
	router *gin.Engine,
	bus *market.Bus,
){

	router.GET(
	"/ws/market",
	func(c *gin.Context){


		conn,err:=
		upgrader.Upgrade(
			c.Writer,
			c.Request,
			nil,
		)


		if err!=nil{
			return
		}


		defer conn.Close()



		ch:=bus.Subscribe()



		for event:=range ch{


			conn.WriteJSON(event)


		}


	},
	)

}