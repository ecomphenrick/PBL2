package main

import (
	"errors"
	"fmt"
)

type Trecho struct {
	Origem   string
	Destino  string
	Assentos int
}

type Carona struct {
	ID        string
	Motorista string
	Trechos   []Trecho
}

func validarTrecho(t Trecho) error {
	if t.Origem == t.Destino {
		return errors.New("Origem e destinos iguais")
	}
	if t.Assentos <= 0 {
		return errors.New("Trecho sem assentos")
	}
	return nil
}

func main() {
	c := Carona{
		ID:        "VJ-1",
		Motorista: "Rick",
		Trechos: []Trecho{
			{Origem: "Salvador", Destino: "Feira de Santana", Assentos: 3},
			{Origem: "Feira de Santana", Destino: "Vitoria da Conquista", Assentos: 3},
		},
	}
	fmt.Printf("%+v\n", c)
	for _, t := range c.Trechos {
		err := validarTrecho(t)
		if err != nil {
			fmt.Println("Trecho inválido:", err)
		} else {
			fmt.Println("Trecho ok:", t.Origem, "->", t.Destino)
		}
	}

}
