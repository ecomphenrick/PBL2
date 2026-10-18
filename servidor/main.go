package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"sync"
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

var trava sync.Mutex

const arquivo = "caronas.json"

func validarTrecho(t Trecho) error {
	if t.Origem == t.Destino {
		return errors.New("Origem e destinos iguais")
	}
	if t.Assentos <= 0 {
		return errors.New("Trecho sem assentos")
	}
	return nil
}

// salvar grava a lista de caronas no arquivo (o armazenamento desta empresa).
// Deve ser chamada com a trava já pega.
func salvar() error {
	dados, err := json.MarshalIndent(caronas, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(arquivo, dados, 0644)
}

// carregar lê o arquivo ao iniciar o servidor. Se ele ainda não existe
// (primeira execução), grava as caronas iniciais para criá-lo.
func carregar() error {
	dados, err := os.ReadFile(arquivo)
	if errors.Is(err, os.ErrNotExist) {
		return salvar()
	}
	if err != nil {
		return err
	}
	return json.Unmarshal(dados, &caronas)
}

func reservar(idCarona string, trecho int) error {
	trava.Lock()
	defer trava.Unlock()

	for i := range caronas {
		if caronas[i].ID != idCarona {
			continue //não é essa carona que eu quero, passe para a proxima
		}
		if trecho < 0 || trecho >= len(caronas[i].Trechos) { //trecho menor q zero ou maior q a qtd = inexistente
			return errors.New("trecho inexistente")
		}
		if caronas[i].Trechos[trecho].Assentos <= 0 {
			return errors.New("sem assentos livres")
		}
		caronas[i].Trechos[trecho].Assentos-- //diminui um assento.
		if err := salvar(); err != nil {
			caronas[i].Trechos[trecho].Assentos++ //não gravou: desfaz na memória
			return err
		}
		return nil
	}
	return errors.New("carona não encontrada")
}

func listarCaronas(w http.ResponseWriter, r *http.Request) {
	trava.Lock()
	defer trava.Unlock()
	w.Header().Set("Content-Type", "application/json") //<- O que vai aqui dentro é Json.
	json.NewEncoder(w).Encode(caronas)                 //traduz para json e coloca em w.
}

func main() {
	if err := carregar(); err != nil {
		fmt.Println("Erro ao carregar as caronas:", err)
		return
	}
	fmt.Println("Caronas carregadas:", len(caronas))

	http.HandleFunc("GET /caronas", listarCaronas) //método e caminho, chamando a função depois que chega um pedido

	fmt.Println("Servidor ouvindo na porta 8080")
	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		fmt.Println("Erro ao iniciar o servidor:", err)
	}
}
