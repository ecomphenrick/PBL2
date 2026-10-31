package main

// busca.go: busca distribuída (barema item 3).
// Estratégia: consulta em TEMPO REAL. A cada busca, pergunta a cada empresa
// as caronas dela (GET /caronas), sem guardar cópia. Empresa fora do ar é
// pulada e o passageiro é avisado; as outras continuam aparecendo.
//
// Combinações: busca num grafo de cidades (até 3 caronas por itinerário).

import (
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/ecomphenrick/PBL2/protocolo"
)

// buscar devolve as opções encontradas e a lista de empresas que não responderam.
func buscar(origem, destino, data string) ([]protocolo.Opcao, []string) {
	// Caronas desta empresa (cópia feita com a trava pega).
	trava.Lock()
	todas := []Carona{}
	for _, c := range estado.Caronas {
		if c.Data == data {
			todas = append(todas, copiar(c))
		}
	}
	trava.Unlock()

	// Caronas das outras empresas, todas ao mesmo tempo.
	var (
		espera   sync.WaitGroup
		protege  sync.Mutex // protege 'todas' e 'foraDoAr' entre as goroutines
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

// cidades devolve a rota da carona: origem do 1º trecho + destino de cada trecho.
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
	maxCaronas = 3  // no máximo 3 caronas (2 conexões) por itinerário
	maxOpcoes  = 15 // quantas opções mostrar ao passageiro
)

// combinar procura itinerários num GRAFO: cada cidade é um vértice e cada
// pedaço de carona com assento livre é uma aresta. Busca em profundidade a
// partir da origem, sem repetir cidade nem carona, até maxCaronas caronas.
func combinar(caronas []Carona, origem, destino string) []protocolo.Opcao {
	var (
		opcoes    []protocolo.Opcao
		caminho   []protocolo.Item    // caronas escolhidas até agora
		visitadas = map[string]bool{} // cidades já usadas neste caminho
		usadas    = map[string]bool{} // caronas já usadas neste caminho
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
				// Desce pela carona enquanto houver assento em cada perna.
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

	// Menos caronas primeiro; depois a mais barata.
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

// montarOpcao calcula o preço e o resumo legível de um itinerário.
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
