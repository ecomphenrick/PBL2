// Cliente do VaiJunto (motorista e passageiro), adaptado do Problema 1.
// Fala SÓ com o servidor da própria empresa, por TCP.
//
// Endereço do servidor: variável VAIJUNTO_SERVIDOR (padrão localhost:9000).
package main

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

const (
	prazoConexao  = 5 * time.Second
	prazoResposta = 15 * time.Second // reservar envolve outras empresas (2PC)
)

type cliente struct {
	conexao  net.Conn
	servidor *bufio.Reader
	teclado  *bufio.Reader
}

func main() {
	endereco := "localhost:9000"
	if v := os.Getenv("VAIJUNTO_SERVIDOR"); v != "" {
		endereco = v
	}
	conexao, err := net.DialTimeout("tcp", endereco, prazoConexao)
	if err != nil {
		fmt.Println("nao consegui conectar em", endereco, ":", err)
		return
	}
	defer conexao.Close()

	c := &cliente{conexao: conexao, servidor: bufio.NewReader(conexao), teclado: bufio.NewReader(os.Stdin)}
	c.mostrar(c.pedir(protocolo.Pedido{Acao: "ping"}))

	for {
		tipo, ok := c.entrada()
		if !ok {
			return
		}
		if tipo == "motorista" {
			c.menuMotorista()
		} else {
			c.menuPassageiro()
		}
		c.pedir(protocolo.Pedido{Acao: "sair"})
	}
}

func (c *cliente) entrada() (string, bool) {
	for {
		menu("VAIJUNTO", "1) entrar", "2) criar conta", "0) sair")
		switch c.opcao() {
		case "1":
			usuario, _ := c.ler("usuario: ")
			senha, _ := c.ler("senha: ")
			if tipo, ok := c.autenticar(usuario, senha); ok {
				return tipo, true
			}
		case "2":
			if tipo, ok := c.criarConta(); ok {
				return tipo, true
			}
		case "0", "":
			return "", false
		default:
			fmt.Println("  opcao invalida")
		}
	}
}

func (c *cliente) criarConta() (string, bool) {
	usuario, ok := c.lerValido("novo usuario: ", nenhumaRegra)
	if !ok {
		return "", false
	}
	senha, ok := c.lerValido("senha: ", nenhumaRegra)
	if !ok {
		return "", false
	}
	fmt.Println("  1) motorista   2) passageiro")
	escolha, ok := c.lerValido("tipo: ", func(t string) error {
		if t != "1" && t != "2" {
			return fmt.Errorf("escolha 1 ou 2")
		}
		return nil
	})
	if !ok {
		return "", false
	}
	tipo := "motorista"
	if escolha == "2" {
		tipo = "passageiro"
	}
	r, err := c.pedir(protocolo.Pedido{Acao: "registrar", Usuario: usuario, Senha: senha, Tipo: tipo})
	if err != nil || !r.OK {
		c.mostrar(r, err)
		return "", false
	}
	fmt.Println(" ", r.Mensagem)
	return c.autenticar(usuario, senha)
}

func (c *cliente) autenticar(usuario, senha string) (string, bool) {
	r, err := c.pedir(protocolo.Pedido{Acao: "login", Usuario: usuario, Senha: senha})
	if err != nil || !r.OK {
		c.mostrar(r, err)
		return "", false
	}
	fmt.Println(" ", r.Mensagem)
	return r.Tipo, true
}

func (c *cliente) menuMotorista() {
	for {
		menu("MOTORISTA", "1) publicar carona", "2) minhas caronas e passageiros", "0) voltar")
		switch c.opcao() {
		case "1":
			c.publicarCarona()
		case "2":
			c.listar(protocolo.Pedido{Acao: "minhas_caronas"}, "voce ainda nao tem caronas")
		case "0", "":
			return
		default:
			fmt.Println("  opcao invalida")
		}
	}
}

func (c *cliente) publicarCarona() {
	fmt.Println("  (Enter vazio cancela)")
	texto, ok := c.lerValido("rota (cidades separadas por virgula): ", validarRota)
	if !ok {
		return
	}
	data, ok := c.lerValido("data (AAAA-MM-DD): ", validarData)
	if !ok {
		return
	}
	assentos, ok := c.lerInteiro("assentos: ", 1)
	if !ok {
		return
	}
	preco, ok := c.lerInteiro("preco por trecho: ", 0)
	if !ok {
		return
	}
	c.mostrar(c.pedir(protocolo.Pedido{Acao: "cadastrar", Rota: separarRota(texto), Data: data, Assentos: assentos, Preco: preco}))
}

func (c *cliente) menuPassageiro() {
	for {
		menu("PASSAGEIRO", "1) buscar e reservar", "2) minhas reservas", "0) voltar")
		switch c.opcao() {
		case "1":
			c.buscarEReservar()
		case "2":
			c.listar(protocolo.Pedido{Acao: "minhas_reservas"}, "voce ainda nao tem reservas")
		case "0", "":
			return
		default:
			fmt.Println("  opcao invalida")
		}
	}
}

func (c *cliente) buscarEReservar() {
	fmt.Println("  (Enter vazio cancela)")
	origem, ok := c.lerValido("origem: ", nenhumaRegra)
	if !ok {
		return
	}
	destino, ok := c.lerValido("destino: ", nenhumaRegra)
	if !ok {
		return
	}
	data, ok := c.lerValido("data (AAAA-MM-DD): ", validarData)
	if !ok {
		return
	}
	r, err := c.pedir(protocolo.Pedido{Acao: "buscar", Origem: origem, Destino: destino, Data: data})
	if err != nil || !r.OK {
		c.mostrar(r, err)
		return
	}
	fmt.Println(" ", r.Mensagem)
	if len(r.Opcoes) == 0 {
		return
	}
	for i, o := range r.Opcoes {
		fmt.Printf("  %d) %s | total R$%d\n", i+1, o.Resumo, o.Preco)
	}
	escolha, ok := c.lerInteiro("reservar qual? (0 cancela): ", 0)
	if !ok || escolha == 0 {
		return
	}
	if escolha > len(r.Opcoes) {
		fmt.Println("  opcao invalida")
		return
	}
	fmt.Println("  reservando (todas as empresas do itinerario precisam aceitar)...")
	c.mostrar(c.pedir(protocolo.Pedido{Acao: "reservar", Itens: r.Opcoes[escolha-1].Itens}))
}

// pedir envia um pedido e espera a resposta, com prazo.
func (c *cliente) pedir(p protocolo.Pedido) (protocolo.Resposta, error) {
	c.conexao.SetDeadline(time.Now().Add(prazoResposta))
	if err := protocolo.EnviarPedido(c.conexao, p); err != nil {
		return protocolo.Resposta{}, err
	}
	return protocolo.LerResposta(c.servidor)
}

func (c *cliente) listar(p protocolo.Pedido, seVazio string) {
	r, err := c.pedir(p)
	if err != nil || !r.OK {
		c.mostrar(r, err)
		return
	}
	if len(r.Linhas) == 0 {
		fmt.Println(" ", seVazio)
		return
	}
	for _, linha := range r.Linhas {
		fmt.Println("  " + linha)
	}
}

func (c *cliente) mostrar(r protocolo.Resposta, err error) {
	switch {
	case err != nil:
		fmt.Println("  conexao perdida:", err)
	case r.OK:
		fmt.Println(" ", r.Mensagem)
	default:
		fmt.Println("  erro:", r.Erro)
	}
}

func menu(titulo string, opcoes ...string) {
	linha := strings.Repeat("=", 36)
	fmt.Println("\n" + linha + "\n  " + titulo + "\n" + linha)
	for _, o := range opcoes {
		fmt.Println("  " + o)
	}
	fmt.Println(linha)
}

func (c *cliente) opcao() string {
	escolha, ok := c.ler("> ")
	if !ok {
		return "0"
	}
	return escolha
}

// ler mostra o rótulo e lê uma linha do teclado (false = entrada acabou).
func (c *cliente) ler(rotulo string) (string, bool) {
	fmt.Print(rotulo)
	texto, err := c.teclado.ReadString('\n')
	if err != nil {
		return "", false
	}
	return strings.TrimSpace(texto), true
}

// lerValido pergunta até a resposta passar na regra. Enter vazio desiste.
func (c *cliente) lerValido(rotulo string, regra func(string) error) (string, bool) {
	for {
		texto, ok := c.ler(rotulo)
		if !ok || texto == "" {
			fmt.Println("  operacao cancelada")
			return "", false
		}
		if err := regra(texto); err != nil {
			fmt.Println("  " + err.Error())
			continue
		}
		return texto, true
	}
}

func (c *cliente) lerInteiro(rotulo string, minimo int) (int, bool) {
	texto, ok := c.lerValido(rotulo, func(t string) error {
		n, err := strconv.Atoi(t)
		if err != nil || n < minimo {
			return fmt.Errorf("precisa ser um numero inteiro >= %d", minimo)
		}
		return nil
	})
	if !ok {
		return 0, false
	}
	n, _ := strconv.Atoi(texto)
	return n, true
}

func nenhumaRegra(string) error { return nil }

func separarRota(texto string) []string {
	var rota []string
	for _, cidade := range strings.Split(texto, ",") {
		if cidade = strings.TrimSpace(cidade); cidade != "" {
			rota = append(rota, cidade)
		}
	}
	return rota
}

func validarRota(texto string) error {
	if len(separarRota(texto)) < 2 {
		return fmt.Errorf("a rota precisa de pelo menos 2 cidades, separadas por virgula")
	}
	return nil
}

func validarData(texto string) error {
	if _, err := time.Parse("2006-01-02", texto); err != nil {
		return fmt.Errorf("data invalida: use AAAA-MM-DD (ex.: 2026-11-03)")
	}
	return nil
}
