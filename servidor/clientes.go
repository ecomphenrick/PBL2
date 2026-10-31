package main

// clientes.go: atendimento dos clientes motorista/passageiro DESTA empresa,
// por socket TCP (adaptado do Problema 1). O cliente nunca fala com outra
// empresa: quem fala com as outras é este servidor, pela API REST.

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strings"
	"time"

	"github.com/ecomphenrick/PBL2/protocolo"
)

var (
	tempoOcioso  = 5 * time.Minute  // cliente mudo esse tempo -> fecha a conexão
	prazoEscrita = 10 * time.Second // prazo para entregar uma resposta
)

// ouvirClientes aceita conexões TCP para sempre, uma goroutine por cliente.
func ouvirClientes(porta string) {
	ouvinte, err := net.Listen("tcp", ":"+porta)
	if err != nil {
		log.Fatalf("erro ao abrir a porta TCP %s: %v", porta, err)
	}
	log.Printf("clientes (TCP) na porta %s", porta)
	for {
		conexao, err := ouvinte.Accept()
		if err != nil {
			continue
		}
		go atender(conexao)
	}
}

func atender(conexao net.Conn) {
	defer conexao.Close()
	leitor := bufio.NewReader(conexao)
	var logado Usuario // quem está logado nesta conexão

	for {
		conexao.SetReadDeadline(time.Now().Add(tempoOcioso))
		pedido, err := protocolo.LerPedido(leitor)

		// Linha chegou, mas não é JSON válido: avisa e continua atendendo.
		if errors.Is(err, protocolo.ErrMensagemInvalida) {
			if responder(conexao, falha(err.Error())) != nil {
				return
			}
			continue
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && !errors.Is(err, os.ErrDeadlineExceeded) {
				log.Printf("conexao com %v caiu: %v", conexao.RemoteAddr(), err)
			}
			return
		}

		if responder(conexao, executar(pedido, &logado)) != nil {
			return
		}
	}
}

func responder(conexao net.Conn, r protocolo.Resposta) error {
	conexao.SetWriteDeadline(time.Now().Add(prazoEscrita))
	return protocolo.EnviarResposta(conexao, r)
}

func falha(mensagem string) protocolo.Resposta {
	return protocolo.Resposta{OK: false, Erro: mensagem}
}

// executar olha a ação do pedido e chama a função certa.
func executar(p protocolo.Pedido, logado *Usuario) protocolo.Resposta {
	if p.Acao != "ping" && p.Acao != "login" && p.Acao != "registrar" && logado.Login == "" {
		return falha("faca login primeiro")
	}

	switch p.Acao {
	case "ping":
		return protocolo.Resposta{OK: true, Mensagem: "pong da empresa " + estado.Empresa}

	case "registrar":
		if err := registrar(p.Usuario, p.Senha, p.Tipo); err != nil {
			return falha(err.Error())
		}
		return protocolo.Resposta{OK: true, Mensagem: fmt.Sprintf("conta criada para %s (%s)", p.Usuario, p.Tipo)}

	case "login":
		trava.Lock()
		u, existe := estado.Usuarios[p.Usuario]
		trava.Unlock()
		if !existe || u.Senha != p.Senha {
			return falha("usuario ou senha invalidos")
		}
		*logado = u
		return protocolo.Resposta{OK: true, Tipo: u.Tipo,
			Mensagem: fmt.Sprintf("bem-vindo, %s (empresa %s)", u.Login, estado.Empresa)}

	case "sair":
		*logado = Usuario{}
		return protocolo.Resposta{OK: true, Mensagem: "sessao encerrada"}

	case "cadastrar":
		if logado.Tipo != "motorista" {
			return falha("so motorista cadastra carona")
		}
		id, err := cadastrarCarona(logado.Login, p.Rota, p.Data, p.Assentos, p.Preco)
		if err != nil {
			return falha(err.Error())
		}
		return protocolo.Resposta{OK: true, CaronaID: id, Mensagem: "carona " + id + " publicada"}

	case "minhas_caronas":
		return protocolo.Resposta{OK: true, Linhas: minhasCaronas(logado.Login)}

	case "buscar":
		if _, err := time.Parse("2006-01-02", p.Data); err != nil {
			return falha("data invalida: use AAAA-MM-DD")
		}
		opcoes, foraDoAr := buscar(p.Origem, p.Destino, p.Data)
		msg := fmt.Sprintf("%d opcao(oes) encontrada(s)", len(opcoes))
		if len(foraDoAr) > 0 {
			msg += "; empresa(s) fora do ar: " + strings.Join(foraDoAr, ", ")
		}
		return protocolo.Resposta{OK: true, Opcoes: opcoes, Mensagem: msg}

	case "reservar":
		if logado.Tipo != "passageiro" {
			return falha("so passageiro reserva")
		}
		tx, err := reservarItinerario(logado.Login, p.Itens)
		if err != nil {
			return protocolo.Resposta{OK: false, Transacao: tx, Erro: "reserva NAO feita: " + err.Error()}
		}
		return protocolo.Resposta{OK: true, Transacao: tx, Mensagem: "itinerario confirmado (" + tx + ")"}

	case "minhas_reservas":
		return protocolo.Resposta{OK: true, Linhas: minhasReservas(logado.Login)}

	default:
		return falha(fmt.Sprintf("acao desconhecida: %q", p.Acao))
	}
}

func registrar(login, senha, tipo string) error {
	login = strings.TrimSpace(login)
	if login == "" || senha == "" {
		return errors.New("informe usuario e senha")
	}
	if tipo != "motorista" && tipo != "passageiro" {
		return errors.New(`o tipo precisa ser "motorista" ou "passageiro"`)
	}
	trava.Lock()
	defer trava.Unlock()
	if _, existe := estado.Usuarios[login]; existe {
		return fmt.Errorf("o usuario %q ja existe", login)
	}
	estado.Usuarios[login] = Usuario{Login: login, Senha: senha, Tipo: tipo}
	if err := salvar(); err != nil {
		delete(estado.Usuarios, login)
		return err
	}
	return nil
}

func cadastrarCarona(motorista string, rota []string, data string, assentos, preco int) (string, error) {
	if len(rota) < 2 {
		return "", errors.New("a rota precisa de pelo menos 2 cidades")
	}
	if assentos < 1 {
		return "", errors.New("a carona precisa de pelo menos 1 assento")
	}
	if _, err := time.Parse("2006-01-02", data); err != nil {
		return "", errors.New("data invalida: use AAAA-MM-DD")
	}
	c := Carona{Motorista: motorista, Data: data, Preco: preco, Total: assentos}
	for i := 0; i+1 < len(rota); i++ {
		t := Trecho{Origem: rota[i], Destino: rota[i+1], Assentos: assentos, Passageiros: []string{}}
		if strings.EqualFold(t.Origem, t.Destino) {
			return "", errors.New("origem e destino iguais em um trecho")
		}
		c.Trechos = append(c.Trechos, t)
	}

	trava.Lock()
	defer trava.Unlock()
	estado.ProxCarona++
	c.ID = fmt.Sprintf("%s-%d", estado.Empresa, estado.ProxCarona)
	estado.Caronas = append(estado.Caronas, c)
	if err := salvar(); err != nil {
		estado.Caronas = estado.Caronas[:len(estado.Caronas)-1]
		estado.ProxCarona--
		return "", err
	}
	log.Printf("carona %s publicada por %s", c.ID, motorista)
	return c.ID, nil
}

func minhasCaronas(motorista string) []string {
	trava.Lock()
	defer trava.Unlock()
	var linhas []string
	for _, c := range estado.Caronas {
		if c.Motorista != motorista {
			continue
		}
		linhas = append(linhas, fmt.Sprintf("carona %s | %s | %s | R$%d por trecho",
			c.ID, c.Data, strings.Join(cidades(c), " -> "), c.Preco))
		for _, t := range c.Trechos {
			linhas = append(linhas, fmt.Sprintf("   %s -> %s: %d livre(s) de %d; passageiros: %s",
				t.Origem, t.Destino, t.Assentos, c.Total, strings.Join(t.Passageiros, ", ")))
		}
	}
	return linhas
}

func minhasReservas(passageiro string) []string {
	trava.Lock()
	defer trava.Unlock()
	var linhas []string
	for _, tx := range estado.Transacoes {
		if tx.Passageiro != passageiro {
			continue
		}
		var itens []string
		for _, p := range tx.Partes {
			for _, it := range p.Itens {
				itens = append(itens, fmt.Sprintf("%s (trechos %d a %d)", it.Carona, it.De, it.Ate-1))
			}
		}
		situacao := map[string]string{perguntando: "em andamento", confirmar: "CONFIRMADA", cancelar: "cancelada"}[tx.Estado]
		linha := fmt.Sprintf("reserva %s | %s | %s", tx.ID, situacao, strings.Join(itens, " + "))
		if tx.Motivo != "" {
			linha += " | motivo: " + tx.Motivo
		}
		linhas = append(linhas, linha)
	}
	return linhas
}
