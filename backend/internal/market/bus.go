package market


type Bus struct {

	subscribers []chan MarketEvent

}


func NewBus()*Bus{

	return &Bus{
		subscribers:
		make([]chan MarketEvent,0),
	}

}



func (b *Bus) Subscribe() <-chan MarketEvent {

	ch:=make(chan MarketEvent,100)

	b.subscribers=
	append(
		b.subscribers,
		ch,
	)

	return ch

}



func (b *Bus) Publish(
	event MarketEvent,
){

	for _,sub:=range b.subscribers{

		select{

		case sub <- event:

		default:

		}

	}

}