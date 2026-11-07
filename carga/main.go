// Simulador de carga e verificador das invariantes do VaiJunto.
//
//	go run ./carga -modo semear      publica caronas (motoristas) em cada empresa
//	go run ./carga -modo carga       muitos passageiros reservando ao mesmo tempo
//	go run ./carga -modo cruzado     reservas cruzadas: mesmos trechos, ordens opostas
//	go run ./carga -modo verificar   lê o estado de todas as empresas e confere
//	                                 as 3 invariantes (sai com código 1 se falhar)
//
// Os passageiros simulados usam o mesmo TCP dos clientes reais.
package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

var (
	modo        = flag.String("modo", "carga", "semear | carga | cruzado | verificar")
	rodadas     = flag.Int("rodadas", 20, "modo cruzado: quantas disputas")
	clientes    = flag.String("clientes", "A=localhost:9001,B=localhost:9002,C=localhost:9003", "porta TCP de cada empresa")
	api         = flag.String("api", "A=http://localhost:8081,B=http://localhost:8082,C=http://localhost:8083", "API REST de cada empresa")
	passageiros = flag.Int("passageiros", 30, "quantos passageiros simultâneos")
	data        = flag.String("data", "2026-11-03", "data das caronas")
)

func main() {
	flag.Parse()
	switch *modo {
	case "semear":
		semear()
	case "carga":
		carga()
	case "cruzado":
		if !cruzado() {
			os.Exit(1)
		}
	case "verificar":
		if !verificar() {
			os.Exit(1)
		}
	default:
		fmt.Println("modo desconhecido:", *modo)
		os.Exit(2)
	}
}

// mapa transforma "A=x,B=y" em {A:x, B:y}.
func mapa(texto string) map[string]string {
	m := map[string]string{}
	for _, item := range strings.Split(texto, ",") {
		if k, v, ok := strings.Cut(strings.TrimSpace(item), "="); ok {
			m[k] = v
		}
	}
	return m
}

func empresas(m map[string]string) []string {
	var lista []string
	for k := range m {
		lista = append(lista, k)
	}
	sort.Strings(lista)
	return lista
}

// ---------- conexão TCP de um cliente simulado ----------

type sessao struct {
	conexao net.Conn
	leitor  *bufio.Reader
}

func conectar(endereco string) (*sessao, error) {
	c, err := net.DialTimeout("tcp", endereco, 5*time.Second)
	if err != nil {
		return nil, err
	}
	return &sessao{conexao: c, leitor: bufio.NewReader(c)}, nil
}

func (s *sessao) pedir(p protocolo.Pedido) (protocolo.Resposta, error) {
	s.conexao.SetDeadline(time.Now().Add(20 * time.Second))
	if err := protocolo.EnviarPedido(s.conexao, p); err != nil {
		return protocolo.Resposta{}, err
	}
	return protocolo.LerResposta(s.leitor)
}

func (s *sessao) entrar(usuario, tipo string) error {
	s.pedir(protocolo.Pedido{Acao: "registrar", Usuario: usuario, Senha: "123", Tipo: tipo}) // pode já existir
	r, err := s.pedir(protocolo.Pedido{Acao: "login", Usuario: usuario, Senha: "123"})
	if err != nil {
		return err
	}
	if !r.OK {
		return fmt.Errorf("login: %s", r.Erro)
	}
	return nil
}

// ---------- semear ----------

// Cenário: Salvador -> Feira -> Conquista -> Ilheus, cada pedaço de uma
// empresa, com poucos assentos para forçar disputa.
var cenario = map[string][][]string{
	"A": {{"Salvador", "Feira"}},
	"B": {{"Feira", "Conquista"}},
	"C": {{"Conquista", "Ilheus"}, {"Salvador", "Conquista"}},
}

func semear() {
	enderecos := mapa(*clientes)
	for _, e := range empresas(enderecos) {
		s, err := conectar(enderecos[e])
		if err != nil {
			fmt.Printf("empresa %s fora do ar: %v\n", e, err)
			continue
		}
		if err := s.entrar("motorista-"+e, "motorista"); err != nil {
			fmt.Printf("empresa %s: %v\n", e, err)
			continue
		}
		for _, rota := range cenario[e] {
			r, _ := s.pedir(protocolo.Pedido{Acao: "cadastrar", Rota: rota, Data: *data, Assentos: 2, Preco: 20})
			fmt.Printf("empresa %s: %s %s\n", e, strings.Join(rota, " -> "), r.Mensagem+r.Erro)
		}
		s.conexao.Close()
	}
}

// ---------- carga ----------

var viagens = [][2]string{
	{"Salvador", "Ilheus"},    // 3 empresas (A + B + C) ou C + C
	{"Salvador", "Conquista"}, // A + B ou C direto
	{"Feira", "Ilheus"},       // B + C
}

func carga() {
	enderecos := mapa(*clientes)
	lista := empresas(enderecos)
	var (
		espera    sync.WaitGroup
		protege   sync.Mutex
		contagem  = map[string]int{}
		largada   = make(chan struct{}) // todos começam juntos
		inicio    time.Time
		resultado []time.Duration
	)
	for i := 0; i < *passageiros; i++ {
		espera.Add(1)
		go func() {
			defer espera.Done()
			empresa := lista[i%len(lista)]
			viagem := viagens[i%len(viagens)]
			situacao, demora := umPassageiro(enderecos[empresa], fmt.Sprintf("pass-%s-%d", empresa, i), viagem, largada)
			protege.Lock()
			contagem[situacao]++
			resultado = append(resultado, demora)
			protege.Unlock()
		}()
	}
	time.Sleep(500 * time.Millisecond) // dá tempo de todos conectarem e logarem
	inicio = time.Now()
	close(largada)
	espera.Wait()
	total := time.Since(inicio)

	sort.Slice(resultado, func(i, j int) bool { return resultado[i] < resultado[j] })
	fmt.Printf("\n%d passageiros em %s\n", *passageiros, total.Round(time.Millisecond))
	for _, k := range []string{"confirmada", "recusada", "sem opcao", "erro"} {
		fmt.Printf("  %-11s %d\n", k, contagem[k])
	}
	if len(resultado) > 0 {
		fmt.Printf("  tempo por reserva: mediana %s, pior %s\n",
			resultado[len(resultado)/2].Round(time.Millisecond), resultado[len(resultado)-1].Round(time.Millisecond))
	}
}

func umPassageiro(endereco, nome string, viagem [2]string, largada chan struct{}) (string, time.Duration) {
	s, err := conectar(endereco)
	if err != nil {
		<-largada
		return "erro", 0
	}
	defer s.conexao.Close()
	if err := s.entrar(nome, "passageiro"); err != nil {
		<-largada
		return "erro", 0
	}
	<-largada // espera o sinal: todos disparam ao mesmo tempo
	comeco := time.Now()
	r, err := s.pedir(protocolo.Pedido{Acao: "buscar", Origem: viagem[0], Destino: viagem[1], Data: *data})
	if err != nil {
		return "erro", time.Since(comeco)
	}
	if len(r.Opcoes) == 0 {
		return "sem opcao", time.Since(comeco)
	}
	r, err = s.pedir(protocolo.Pedido{Acao: "reservar", Itens: r.Opcoes[0].Itens})
	switch {
	case err != nil:
		return "erro", time.Since(comeco)
	case r.OK:
		return "confirmada", time.Since(comeco)
	default:
		return "recusada", time.Since(comeco)
	}
}

// ---------- cruzado ----------

// cruzado: B e C publicam uma carona de 1 assento cada. Um passageiro pede
// [B, C] e outro pede [C, B] ao mesmo tempo. O sistema não pode travar e
// no máximo um confirma (às vezes os dois são recusados).
func cruzado() bool {
	enderecos := mapa(*clientes)
	motoB, err1 := conectarComo(enderecos["B"], "motorista-B", "motorista")
	motoC, err2 := conectarComo(enderecos["C"], "motorista-C", "motorista")
	pass1, err3 := conectarComo(enderecos["A"], "cruzado-A", "passageiro")
	pass2, err4 := conectarComo(enderecos["C"], "cruzado-C", "passageiro")
	for _, err := range []error{err1, err2, err3, err4} {
		if err != nil {
			fmt.Println("FALHA ao conectar:", err)
			return false
		}
	}

	contagem := map[int]int{} // quantos confirmaram na rodada -> quantas rodadas
	travou := 0
	var pior time.Duration
	inicio := time.Now()
	for i := 0; i < *rodadas; i++ {
		b, _ := motoB.pedir(protocolo.Pedido{Acao: "cadastrar", Rota: []string{"Feira", "Conquista"}, Data: *data, Assentos: 1, Preco: 20})
		c, _ := motoC.pedir(protocolo.Pedido{Acao: "cadastrar", Rota: []string{"Conquista", "Ilheus"}, Data: *data, Assentos: 1, Preco: 20})
		itemB := protocolo.Item{Carona: b.CaronaID, De: 0, Ate: 1}
		itemC := protocolo.Item{Carona: c.CaronaID, De: 0, Ate: 1}

		var espera sync.WaitGroup
		ok := make([]bool, 2)
		erro := make([]bool, 2)
		comeco := time.Now()
		for j, s := range []*sessao{pass1, pass2} {
			itens := []protocolo.Item{itemB, itemC} // ordem B, C
			if j == 1 {
				itens = []protocolo.Item{itemC, itemB} // ordem oposta: C, B
			}
			espera.Add(1)
			go func() {
				defer espera.Done()
				r, err := s.pedir(protocolo.Pedido{Acao: "reservar", Itens: itens})
				ok[j], erro[j] = err == nil && r.OK, err != nil
			}()
		}
		espera.Wait()
		pior = max(pior, time.Since(comeco))
		if erro[0] || erro[1] {
			travou++ // sem resposta dentro do prazo do cliente (20 s)
		}
		n := 0
		for _, v := range ok {
			if v {
				n++
			}
		}
		contagem[n]++
	}

	fmt.Printf("\n%d rodadas de reservas cruzadas em %s (pior rodada: %s)\n",
		*rodadas, time.Since(inicio).Round(time.Millisecond), pior.Round(time.Millisecond))
	fmt.Printf("  um confirmou:          %d\n", contagem[1])
	fmt.Printf("  os dois recusados:     %d  (aborto desnecessario: efeito colateral do no-wait)\n", contagem[0])
	fmt.Printf("  os dois confirmaram:   %d  (seria venda dupla)\n", contagem[2])
	fmt.Printf("  sem resposta (travou): %d\n", travou)
	if contagem[2] > 0 || travou > 0 {
		fmt.Println("FALHA")
		return false
	}
	fmt.Println("OK: nenhuma rodada travou e nenhuma vendeu o mesmo assento duas vezes")
	return true
}

func conectarComo(endereco, usuario, tipo string) (*sessao, error) {
	s, err := conectar(endereco)
	if err != nil {
		return nil, err
	}
	return s, s.entrar(usuario, tipo)
}

// ---------- verificar ----------

// Só os campos do estado que o verificador usa.
type estadoEmpresa struct {
	Caronas []struct {
		ID      string `json:"id"`
		Total   int    `json:"total"`
		Trechos []struct {
			Origem      string   `json:"origem"`
			Destino     string   `json:"destino"`
			Assentos    int      `json:"assentos"`
			Passageiros []string `json:"passageiros"`
		} `json:"trechos"`
	} `json:"caronas"`
	Participacoes map[string]struct {
		Coordenador string           `json:"coordenador"`
		Itens       []protocolo.Item `json:"itens"`
		Estado      string           `json:"estado"`
	} `json:"participacoes"`
	Transacoes map[string]struct {
		Estado string `json:"estado"`
		Partes []struct {
			Empresa string `json:"empresa"`
		} `json:"partes"`
	} `json:"transacoes"`
}

func verificar() bool {
	enderecos := mapa(*api)
	estados := map[string]estadoEmpresa{}
	cliente := &http.Client{Timeout: 5 * time.Second}
	for _, e := range empresas(enderecos) {
		r, err := cliente.Get(enderecos[e] + "/estado")
		if err != nil {
			fmt.Printf("FALHA: empresa %s fora do ar, nao da para verificar: %v\n", e, err)
			return false
		}
		var est estadoEmpresa
		err = json.NewDecoder(r.Body).Decode(&est)
		r.Body.Close()
		if err != nil {
			fmt.Printf("FALHA: estado da empresa %s ilegivel: %v\n", e, err)
			return false
		}
		estados[e] = est
	}

	var problemas []string
	falhou := func(formato string, args ...any) { problemas = append(problemas, fmt.Sprintf(formato, args...)) }
	confirmadas, provisorias := 0, 0

	for e, est := range estados {
		// Assentos presos em provisório, por trecho.
		presos := map[string]int{}
		for tx, p := range est.Participacoes {
			if p.Estado == "provisorio" {
				provisorias++
				falhou("INVARIANTE 3 (assento bloqueado): %s tem %s provisoria", e, tx)
				for _, it := range p.Itens {
					for t := it.De; t < it.Ate; t++ {
						presos[fmt.Sprintf("%s/%d", it.Carona, t)]++
					}
				}
			}
			if p.Estado == "confirmado" {
				confirmadas++
				coord, ok := estados[p.Coordenador].Transacoes[tx]
				if !ok || coord.Estado != "confirmar" {
					falhou("INVARIANTE 2 (pela metade): %s confirmou %s, mas o coordenador %s diz %q", e, tx, p.Coordenador, coord.Estado)
				}
			}
		}
		// Invariante 1: nenhum assento vendido duas vezes.
		for _, c := range est.Caronas {
			for t, tr := range c.Trechos {
				soma := tr.Assentos + len(tr.Passageiros) + presos[fmt.Sprintf("%s/%d", c.ID, t)]
				if tr.Assentos < 0 || len(tr.Passageiros) > c.Total || soma != c.Total {
					falhou("INVARIANTE 1 (assento duplicado): %s trecho %s->%s: livres %d + confirmados %d + presos %d != total %d",
						c.ID, tr.Origem, tr.Destino, tr.Assentos, len(tr.Passageiros), presos[fmt.Sprintf("%s/%d", c.ID, t)], c.Total)
				}
			}
		}
		// Invariante 2: "confirmar" no coordenador => todas as donas confirmaram.
		for tx, coord := range est.Transacoes {
			for _, parte := range coord.Partes {
				p := estados[parte.Empresa].Participacoes[tx]
				if coord.Estado == "confirmar" && p.Estado != "confirmado" {
					falhou("INVARIANTE 2 (pela metade): %s decidiu confirmar, mas %s esta %q", tx, parte.Empresa, p.Estado)
				}
				if coord.Estado != "confirmar" && p.Estado == "confirmado" {
					falhou("INVARIANTE 2 (pela metade): %s esta %q, mas %s confirmou", tx, coord.Estado, parte.Empresa)
				}
			}
		}
	}

	fmt.Printf("\nverificadas %d empresa(s): %d participacao(oes) confirmada(s), %d provisoria(s)\n",
		len(estados), confirmadas, provisorias)
	if len(problemas) == 0 {
		fmt.Println("OK: as 3 invariantes valem")
		fmt.Println("  1. nenhum assento confirmado para dois passageiros")
		fmt.Println("  2. nenhum itinerario confirmado pela metade")
		fmt.Println("  3. nenhum assento bloqueado")
		return true
	}
	sort.Strings(problemas)
	for _, p := range problemas {
		fmt.Println("FALHA:", p)
	}
	return false
}
