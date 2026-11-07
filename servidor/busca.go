package main

// Busca: pergunta a todas as empresas na hora (sem guardar cópia).
// Empresa fora do ar é pulada e o passageiro é avisado.

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ecomphenrick/PBL2/protocolo"
)

// buscar devolve as opções e as empresas que não responderam.
func buscar(origem, destino, data string) ([]protocolo.Opcao, []string) {
	trava.Lock()
	todas := []Carona{}
	for _, c := range estado.Caronas {
		if c.Data == data {
			todas = append(todas, copiar(c))
		}
	}
	trava.Unlock()

	var (
		espera   sync.WaitGroup
		protege  sync.Mutex // as goroutines escrevem em 'todas' e 'foraDoAr'
		foraDoAr []string
	)
	for empresa := range pares {
		espera.Add(1)
		go func() {
			defer espera.Done()
			var delas []Carona
			_, err := chamar(empresa, "GET", "/caronas?data="+data, nil, &delas)
			protege.Lock()
			defer protege.Unlock()
			if err != nil {
				foraDoAr = append(foraDoAr, empresa)
				return
			}
			todas = append(todas, delas...)
		}()
	}
	espera.Wait()
	sort.Strings(foraDoAr)

	return combinar(todas, origem, destino), foraDoAr
}

func cidades(c Carona) []string {
	if len(c.Trechos) == 0 {
		return nil
	}
	lista := []string{c.Trechos[0].Origem}
	for _, t := range c.Trechos {
		lista = append(lista, t.Destino)
	}
	return lista
}

const (
	maxCaronas = 3
	maxOpcoes  = 15
)

// combinar monta as viagens num grafo: cidade = ponto, trecho com assento = ligação.
// Anda a partir da origem, sem repetir cidade nem carona, até maxCaronas caronas.
func combinar(caronas []Carona, origem, destino string) []protocolo.Opcao {
	var (
		opcoes    []protocolo.Opcao
		caminho   []protocolo.Item
		visitadas = map[string]bool{}
		usadas    = map[string]bool{}
		procurar  func(cidade string)
	)
	procurar = func(cidade string) {
		if len(caminho) == maxCaronas {
			return
		}
		for _, c := range caronas {
			if usadas[c.ID] {
				continue
			}
			rota := cidades(c)
			for i := range rota {
				if !strings.EqualFold(rota[i], cidade) {
					continue
				}
				// Uma perna lotada interrompe a carona dali em diante.
				for j := i + 1; j < len(rota) && c.Trechos[j-1].Assentos > 0; j++ {
					proxima := strings.ToLower(rota[j])
					if visitadas[proxima] {
						break
					}
					caminho = append(caminho, protocolo.Item{Carona: c.ID, De: i, Ate: j})
					if strings.EqualFold(rota[j], destino) {
						opcoes = append(opcoes, montarOpcao(caronas, caminho...))
					} else {
						visitadas[proxima], usadas[c.ID] = true, true
						procurar(rota[j])
						visitadas[proxima], usadas[c.ID] = false, false
					}
					caminho = caminho[:len(caminho)-1]
				}
			}
		}
	}
	visitadas[strings.ToLower(origem)] = true
	procurar(origem)

	// Menos baldeações primeiro; depois a mais barata.
	sort.SliceStable(opcoes, func(i, j int) bool {
		if len(opcoes[i].Itens) != len(opcoes[j].Itens) {
			return len(opcoes[i].Itens) < len(opcoes[j].Itens)
		}
		return opcoes[i].Preco < opcoes[j].Preco
	})
	if len(opcoes) > maxOpcoes {
		opcoes = opcoes[:maxOpcoes]
	}
	return opcoes
}

func montarOpcao(caronas []Carona, itens ...protocolo.Item) protocolo.Opcao {
	o := protocolo.Opcao{Itens: itens}
	var partes []string
	for _, it := range itens {
		for _, c := range caronas {
			if c.ID != it.Carona {
				continue
			}
			rota := cidades(c)
			o.Preco += c.Preco * (it.Ate - it.De)
			partes = append(partes, fmt.Sprintf("%s [%s] %s -> %s",
				c.ID, c.Motorista, rota[it.De], rota[it.Ate]))
		}
	}
	o.Resumo = strings.Join(partes, " + ")
	return o
}
