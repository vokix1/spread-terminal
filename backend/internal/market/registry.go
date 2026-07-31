package market


type Registry struct {

	instruments map[string]Instrument

}


func NewRegistry()*Registry{

	return &Registry{
		instruments:make(map[string]Instrument),
	}

}



func (r *Registry) Add(
	i Instrument,
){

	r.instruments[i.Exchange+":"+i.Symbol]=i

}



func (r *Registry) All()[]Instrument{

	result:=make([]Instrument,0)

	for _,v:=range r.instruments{

		result=append(result,v)

	}

	return result
}