package main

import "testing"

func carona(id string, assentos int, rota ...string) Carona {
	c := Carona{ID: id, Motorista: "m", Preco: 10, Total: assentos}
	for i := 0; i+1 < len(rota); i++ {
		c.Trechos = append(c.Trechos, Trecho{Origem: rota[i], Destino: rota[i+1], Assentos: assentos})
	}
	return c
}

// Itinerário que passa pelas 3 empresas é encontrado.
func TestBuscaTresEmpresas(t *testing.T) {
	caronas := []Carona{
		carona("A-1", 2, "Salvador", "Feira"),
		carona("B-1", 2, "Feira", "Conquista"),
		carona("C-1", 2, "Conquista", "Ilheus"),
	}
	opcoes := combinar(caronas, "salvador", "Ilheus")
	if len(opcoes) != 1 || len(opcoes[0].Itens) != 3 {
		t.Fatalf("esperava 1 opcao com 3 caronas, veio %+v", opcoes)
	}
}

// Perna lotada não aparece; direta vem antes de conexão.
func TestBuscaPulaLotadaEOrdena(t *testing.T) {
	caronas := []Carona{
		carona("A-1", 0, "Salvador", "Ilheus"), // lotada
		carona("A-2", 1, "Salvador", "Feira"),
		carona("B-1", 1, "Feira", "Ilheus"),
		carona("C-1", 1, "Salvador", "Conquista", "Ilheus"),
	}
	opcoes := combinar(caronas, "Salvador", "Ilheus")
	if len(opcoes) != 2 {
		t.Fatalf("esperava 2 opcoes, veio %d: %+v", len(opcoes), opcoes)
	}
	if opcoes[0].Itens[0].Carona != "C-1" {
		t.Fatalf("a direta deveria vir primeiro: %+v", opcoes[0])
	}
}
