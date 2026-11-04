# API REST entre servidores — VaiJunto

Cada empresa expõe a mesma API (porta padrão `8080`). Os servidores só se comunicam por
ela; os clientes motorista/passageiro falam apenas com o servidor da própria empresa,
por TCP (ver `protocolo/`).

- Formato: JSON (`Content-Type: application/json`) em pedidos e respostas.
- **Timeout:** quem chama espera no máximo **3 s**. Sem resposta, a empresa chamada é
  tratada como fora do ar *naquele pedido*; o servidor segue atendendo o resto.
- **Identificadores:**
  - carona: `<empresa>-<número>`, por exemplo `B-3`. A letra antes do traço indica a empresa
    dona, então o roteamento não precisa de catálogo.
  - transação: `<empresa coordenadora>-T<número>`, por exemplo `A-T7`. É único no
    consórcio e torna os pedidos idempotentes.

## Recursos

| Método | Caminho | Quem chama | Função |
|---|---|---|---|
| GET | `/saude` | qualquer um | a empresa está no ar? |
| GET | `/caronas?data=AAAA-MM-DD` | servidor que busca | caronas desta empresa (busca distribuída) |
| POST | `/transacoes/{id}/preparar` | coordenador | fase 1 do 2PC: separar assento provisório |
| POST | `/transacoes/{id}/confirmar` | coordenador | fase 2: confirmar |
| POST | `/transacoes/{id}/cancelar` | coordenador | fase 2: cancelar e devolver |
| GET | `/transacoes/{id}/decisao` | dono do trecho | qual foi a decisão do coordenador? |
| GET | `/transacoes/{id}/situacao` | dono do trecho | o que outra participante sabe da transação? (coordenador fora do ar) |
| GET | `/estado` | teste/verificador | estado completo desta empresa |
| POST | `/simular-falha?ponto=...` | demonstração | derruba esta empresa num ponto do 2PC |

---

### GET /caronas

Devolve só as caronas **desta** empresa. O parâmetro `data` é opcional e filtra pelo dia.
Nenhum servidor guarda cópia: a cada busca, o servidor do passageiro consulta todas as
empresas ao mesmo tempo e junta o resultado.

**200**
```json
[
  {
    "id": "B-1", "motorista": "joao", "data": "2026-11-03", "preco": 20, "total": 2,
    "trechos": [
      {"origem": "Feira", "destino": "Conquista", "assentos": 1, "passageiros": ["ana"]}
    ]
  }
]
```
`assentos` = livres agora (os provisórios já foram descontados).

---

### POST /transacoes/{id}/preparar  (fase 1)

**Pedido**
```json
{
  "coordenador": "A",
  "passageiro": "ana",
  "itens": [{"carona": "B-1", "de": 0, "ate": 1}],
  "participantes": ["A", "B", "C"]
}
```
`participantes` lista todas as empresas da transação. A participação guarda essa lista
para poder perguntar às outras se o coordenador sumir.
Cada item pede os trechos `de` até `ate-1` de uma carona. A empresa confere **todos** os
itens dentro da trava dela. Se houver assento em todos, desconta e grava a participação
como **provisória** antes de responder. Senão, responde "não" na hora, sem fila.

| Código | Corpo | Significado |
|---|---|---|
| 200 | `{"voto": "sim"}` | assentos separados (provisório) |
| 409 | `{"voto": "nao", "motivo": "sem assento livre em B-1 (Feira -> Conquista)"}` | recusado |
| 400 | `{"erro": "corpo invalido"}` | JSON malformado |

**Repetição:** o mesmo `{id}` de novo devolve a mesma resposta, sem separar assento de
novo. Se um `cancelar` com esse `{id}` já tiver chegado antes (a rede pode inverter a
ordem), o preparar é recusado.

---

### POST /transacoes/{id}/confirmar  (fase 2)

| Código | Significado |
|---|---|
| 200 `{"estado": "confirmado"}` | confirmado (ou já estava: repetição é segura) |
| 409 `{"erro": "..."}` | transação desconhecida ou já cancelada/recusada nesta empresa |
| 500 `{"erro": "..."}` | falha ao gravar; o coordenador reenvia depois |

### POST /transacoes/{id}/cancelar  (fase 2)

| Código | Significado |
|---|---|
| 200 `{"estado": "cancelado"}` | assentos devolvidos (ou já estava cancelado; ou anotado para recusar um preparar atrasado) |
| 409 `{"erro": "..."}` | a transação já estava confirmada (não deve acontecer: a decisão não muda) |
| 500 `{"erro": "..."}` | falha ao gravar; o coordenador reenvia depois |

---

### GET /transacoes/{id}/decisao

Quem chama é o **dono** de um assento provisório cujo prazo (30 s) venceu, e a pergunta
vai ao **coordenador**.

**200** `{"decisao": "confirmar"}` ou `{"decisao": "cancelar"}`

- A transação não é conhecida, ou caiu sem decisão gravada: responde `cancelar` (presume
  cancelado).
- A transação ainda está esperando votos: o coordenador **decide `cancelar` na hora**,
  grava e responde. Assim o dono nunca libera um assento que depois seria confirmado.

### GET /transacoes/{id}/situacao

Terminação cooperativa. Quem chama é um **dono** com assento provisório cujo
coordenador não respondeu. A pergunta vai a outra **participante** da mesma transação.

**200** `{"situacao": "provisorio" | "confirmado" | "cancelado" | "recusado"}`

| Resposta | O que o dono que perguntou faz |
|---|---|
| `confirmado` | confirma: o coordenador decidiu confirmar |
| `cancelado` ou `recusado` | cancela: com um "não" (ou um cancelar), o coordenador não pode ter confirmado |
| `provisorio` | nada: só o coordenador sabe; continua segurando e pergunta de novo |

Se a transação nunca chegou a esta empresa, ela anota `cancelado` antes de responder.
Assim um `preparar` atrasado é recusado e o coordenador não consegue mais confirmar.
Repetir a pergunta devolve a mesma resposta.

---

### GET /estado

Devolve o estado completo gravado pela empresa: caronas, participações (lado dono) e
transações (lado coordenador). É usado pelo verificador de invariantes (`carga -modo
verificar`).

### POST /simular-falha?ponto=apos-fase1 | apos-decisao

Só para demonstração. A **próxima** transação coordenada por esta empresa derruba o
processo:
- `apos-fase1`: depois dos votos e antes de gravar a decisão;
- `apos-decisao`: depois de gravar a decisão e antes de avisar as participantes.

---

## O que acontece quando...

| Situação | Comportamento |
|---|---|
| O `preparar` não recebe resposta | conta como "não"; o coordenador cancela todas (inclusive quem não respondeu) |
| O `confirmar`/`cancelar` não recebe resposta | a decisão já está gravada; o coordenador reenvia a cada 5 s até a empresa receber |
| A mesma requisição é repetida | mesmo efeito de uma vez só (idempotência pelo ID da transação) |
| O coordenador cai antes de decidir | ao voltar, decide "cancelar" e avisa todas |
| O coordenador cai depois de decidir | ao voltar, reenvia a decisão gravada |
| O coordenador não responde | o dono pergunta às outras participantes (`/situacao`); se alguma sabe, segue ela |
| O coordenador some e ninguém sabe | quem votou "sim" **não libera sozinho**: segura e pergunta de 5 em 5 s até alguém saber |
| Um dono cai com assento provisório | ao voltar, pergunta a decisão ao coordenador e aplica |

Diagrama de sequência: [`docs/diagramas/sequencia.md`](../diagramas/sequencia.md).
