package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
)

type Trecho struct {
	Origem   string `json:"origem"`
	Destino  string `json:"destino"`
	Assentos int    `json:"assentos"`
}

type Carona struct {
	ID        string   `json:"id"`
	Motorista string   `json:"motorista"`
	Trechos   []Trecho `json:"trechos"`
}

var caronas = []Carona{
	{
		ID:        "VJ-1",
		Motorista: "Rick",
		Trechos: []Trecho{
			{Origem: "Salvador", Destino: "Feira de Santana", Assentos: 3},
			{Origem: "Feira de Santana", Destino: "Vitoria da Conquista", Assentos: 3},
		},
	},
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

func listarCaronas(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json") //<- O que vai aqui dentro é Json.
	json.NewEncoder(w).Encode(caronas)                 //traduz para json e coloca em w.
}

func main() {
	for _, c := range caronas {
		fmt.Printf("%+v\n", c)
		for _, t := range c.Trechos {
			if err := validarTrecho(t); err != nil {
				fmt.Println("Trecho inválido:", err)
			}
		}

		http.HandleFunc("GET /caronas", listarCaronas) //método e caminho, chamando a função depois que chega um pedido

		fmt.Println("Servidor ouvindo na porta 8080")
		err := http.ListenAndServe(":8080", nil)
		if err != nil {
			fmt.Println("Erro ao iniciar o servidor:", err)
		}

	}
}
