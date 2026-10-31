# VaiJunto — consórcio de caronas distribuído (PBL TEC502, Problema 2)

Três ou mais empresas de caronas, cada uma com **seu próprio servidor, contêiner e
armazenamento**, cooperam por uma API REST para vender itinerários com trechos de
empresas diferentes. Não há servidor central, banco compartilhado nem ferramenta de
coordenação pronta: exclusão mútua, atomicidade e recuperação de falhas estão
implementadas no próprio código, em Go.

## Garantias (invariantes)

1. Nenhum assento de um trecho é confirmado para dois passageiros.
2. Nenhum itinerário é confirmado pela metade: ou todos os trechos, ou nenhum.
3. Nenhum assento fica bloqueado depois da queda e da recuperação de um servidor.

## Como funciona (resumo)

| Assunto | Solução | Onde no código |
|---|---|---|
| Dados | Cada empresa guarda só as próprias caronas; das outras, pergunta pela API | `servidor/modelo.go` |
| Busca | Consulta em tempo real a todas as empresas (timeout de 3 s; quem não responde é pulado) e busca num grafo de cidades, até 3 caronas | `servidor/busca.go` |
| Exclusão mútua e precedência | O dono do trecho decide, pela ordem de chegada na trava (`sync.Mutex`) dele; sem relógio | `servidor/dono.go` |
| Deadlock | "Ninguém espera": sem assento livre, o dono responde **não** na hora | `servidor/dono.go` |
| Atomicidade | Commit em duas fases (2PC). Coordenador = empresa do passageiro; participantes = donas dos trechos | `servidor/coordenador.go` |
| Falhas | Timeout em toda chamada; provisório pergunta ao coordenador após 30 s; válvula libera após 5 min | `servidor/falhas.go`, `servidor/rede.go` |
| Persistência | Tudo é gravado em disco **antes** de responder; o coordenador grava a decisão antes do primeiro "confirmar" | `servidor/modelo.go` |
| Recuperação | Ao religar: transação sem decisão vira "cancelar"; decisão gravada é reenviada | `servidor/falhas.go` |

A especificação da API REST está em [`docs/api/API.md`](docs/api/API.md).

## Estrutura

```
servidor/   servidor de uma empresa (API REST + atendimento TCP dos clientes)
cliente/    cliente motorista/passageiro (menu no terminal, TCP)
protocolo/  mensagens cliente <-> servidor (JSON por linha)
carga/      simulador de passageiros e verificador das invariantes
testes/     teste automatizado de carga com queda de empresas
```

## Rodar no mesmo computador (3 empresas)

Requisitos: Docker. Para os testes, também Go 1.27.

```bash
docker compose up -d --build        # sobe as empresas A, B e C
docker compose logs -f              # acompanha o que cada uma faz
```

| Empresa | API REST | Clientes TCP |
|---|---|---|
| A | http://localhost:8081 | localhost:9001 |
| B | http://localhost:8082 | localhost:9002 |
| C | http://localhost:8083 | localhost:9003 |

Abrir um cliente (motorista ou passageiro) na empresa A (no Git Bash do Windows, pôr
`MSYS_NO_PATHCONV=1` antes do comando):

```bash
docker compose run --rm -e VAIJUNTO_SERVIDOR=empresa-a:9000 empresa-a /bin/cliente
```

Provocar e observar uma falha:

```bash
docker compose stop empresa-b       # a empresa B cai
docker compose start empresa-b      # B volta e recupera o que estava em andamento
```

## Testes

```bash
go test ./servidor -v               # testes da lógica (concorrência, idempotência, recuperação)
bash testes/teste_falha.sh          # carga + queda de empresas + verificação das invariantes
```

O `teste_falha.sh` sobe as 3 empresas do zero e faz 4 rodadas: carga sem falha; carga
com a empresa B derrubada no meio; carga com a empresa A derrubada no meio; e o
coordenador caindo logo depois de gravar a decisão. Depois de cada rodada, religa a
empresa, espera o prazo do provisório e confere as 3 invariantes no estado de todas as
empresas.

Comandos avulsos do simulador:

```bash
go run ./carga -modo semear                   # publica caronas de teste
go run ./carga -modo carga -passageiros 90    # passageiros simultâneos
go run ./carga -modo verificar                # confere as 3 invariantes
```

## Laboratório: uma empresa por computador

Repetir em **cada** máquina (exemplo com 3 máquinas; trocar os IPs pelos reais).

**1. Preparar a imagem**

```bash
git clone https://github.com/ecomphenrick/PBL2.git && cd PBL2
docker build -t vaijunto .
hostname -I                          # anotar o IP desta máquina
```

Sem internet no laboratório: gerar a imagem antes, em casa, com
`docker save vaijunto -o vaijunto.tar`, levar o arquivo e carregar com
`docker load -i vaijunto.tar`.

**2. Subir a empresa** (exemplo da máquina 1, empresa A; nas outras, trocar `EMPRESA`,
o nome e os `PARES`)

```bash
docker run -d --name empresa-a -p 8080:8080 -p 9000:9000 \
  -e EMPRESA=A \
  -e PARES="B=http://IP_DA_MAQUINA_2:8080,C=http://IP_DA_MAQUINA_3:8080" \
  -v dados-a:/dados vaijunto
docker logs -f empresa-a
```

**3. Conferir a comunicação** (de cada máquina para as outras)

```bash
curl http://IP_DA_MAQUINA_2:8080/saude
```

Se não responder: liberar as portas 8080 e 9000 no firewall (`sudo ufw allow 8080`,
`sudo ufw allow 9000`) e conferir o IP.

**4. Usar**

```bash
docker run -it --rm -e VAIJUNTO_SERVIDOR=IP_DESTA_MAQUINA:9000 vaijunto /bin/cliente
docker run --rm vaijunto /bin/carga -modo semear \
  -clientes "A=IP1:9000,B=IP2:9000,C=IP3:9000"
docker run --rm vaijunto /bin/carga -modo carga -passageiros 60 \
  -clientes "A=IP1:9000,B=IP2:9000,C=IP3:9000"
docker run --rm vaijunto /bin/carga -modo verificar \
  -api "A=http://IP1:8080,B=http://IP2:8080,C=http://IP3:8080"
```

**5. Falha**

```bash
docker stop empresa-a     # derruba a empresa desta máquina
docker start empresa-a    # religa; os dados estão no volume dados-a
```

Falha num ponto exato do protocolo (vale para a próxima reserva coordenada pela empresa):

```bash
curl -X POST "http://IP1:8080/simular-falha?ponto=apos-fase1"    # cai antes de decidir
curl -X POST "http://IP1:8080/simular-falha?ponto=apos-decisao"  # cai depois de gravar a decisão
```

## Configuração do servidor (variáveis de ambiente)

| Variável | Padrão | Significado |
|---|---|---|
| `EMPRESA` | `A` | letra da empresa (prefixo dos IDs de carona e transação) |
| `PARES` | — | outras empresas: `B=http://host:8080,C=http://host:8080` |
| `PORTA_API` | `8080` | porta da API REST entre servidores |
| `PORTA_CLIENTES` | `9000` | porta TCP dos clientes |
| `DADOS` | `/dados/estado.json` | arquivo de estado (no contêiner) |
| `PRAZO_PROVISORIO` | `30s` | após esse tempo, o dono pergunta a decisão ao coordenador |
| `VALVULA` | `5m` | após esse tempo sem resposta do coordenador, o dono libera o assento |
