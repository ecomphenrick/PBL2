# VaiJunto — consórcio de caronas (TEC502, Problema 2)

Três empresas de caronas, cada uma com seu próprio servidor em Docker, conversando por
API REST. Um passageiro pode reservar uma viagem com trechos de empresas diferentes:
ou reserva tudo, ou nada. Não há servidor central.

- **2PC (commit em duas fases):** a empresa do passageiro coordena; as donas dos trechos votam.
- **Exclusão mútua:** só o dono da carona decide sobre os assentos dela.
- **Deadlock:** sem assento, a resposta é "não" na hora (ninguém espera).
- **Falhas:** toda chamada tem timeout de 3 s. Quem votou "sim" segura o assento e pergunta
  ao coordenador até ele responder; nunca libera sozinho.
- **Recuperação:** tudo é gravado em disco antes de responder.

API REST: [`docs/api/API.md`](docs/api/API.md). Diagramas: [`docs/diagramas/sequencia.md`](docs/diagramas/sequencia.md).

## Rodar no próprio computador

```bash
docker compose up -d --build     # sobe as empresas A, B e C
docker compose logs -f           # mostra o que cada uma faz
```

Cliente (motorista ou passageiro):

```bash
docker compose run --rm -e VAIJUNTO_SERVIDOR=empresa-a:9000 empresa-a /bin/cliente
```

## Testes

```bash
go test ./servidor               # testes da lógica
bash testes/teste_falha.sh       # carga + queda de empresas + confere as 3 regras
```

## Laboratório (uma empresa por máquina)

**1. Em cada máquina:** baixar o projeto, montar a imagem e ver o IP.

```bash
docker build -t vaijunto .
hostname -I
```

**2. Subir a empresa** (trocar IP1, IP2, IP3 pelos IPs reais):

```bash
# máquina 1
docker run -d --name empresa-a -p 8080:8080 -p 9000:9000 -e EMPRESA=A -e PARES="B=http://IP2:8080,C=http://IP3:8080" -v dados-a:/dados vaijunto
# máquina 2
docker run -d --name empresa-b -p 8080:8080 -p 9000:9000 -e EMPRESA=B -e PARES="A=http://IP1:8080,C=http://IP3:8080" -v dados-b:/dados vaijunto
# máquina 3
docker run -d --name empresa-c -p 8080:8080 -p 9000:9000 -e EMPRESA=C -e PARES="A=http://IP1:8080,B=http://IP2:8080" -v dados-c:/dados vaijunto
```

**3. Testar a conexão** (de uma máquina para as outras):

```bash
curl http://IP2:8080/saude
```

Se não responder: `sudo ufw allow 8080` e `sudo ufw allow 9000`.

**4. Usar e testar** (de qualquer máquina):

```bash
C="A=IP1:9000,B=IP2:9000,C=IP3:9000"
docker run --rm vaijunto /bin/carga -modo semear -clientes "$C"
docker run --rm vaijunto /bin/carga -modo carga -clientes "$C"
docker run --rm vaijunto /bin/carga -modo cruzado -clientes "$C" -data 2026-11-07
docker run --rm vaijunto /bin/carga -modo verificar -api "A=http://IP1:8080,B=http://IP2:8080,C=http://IP3:8080"
docker run -it --rm -e VAIJUNTO_SERVIDOR=IP1:9000 vaijunto /bin/cliente
```

**5. Derrubar e religar uma empresa:**

```bash
docker stop empresa-a
docker start empresa-a
```
