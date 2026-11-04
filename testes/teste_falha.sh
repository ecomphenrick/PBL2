#!/usr/bin/env bash
# Teste automatizado: carga + concorrência entre servidores + injeção de falhas.
# Rodar na raiz do projeto (Git Bash no Windows, ou Linux):  bash testes/teste_falha.sh
#
# Rodada 1: muitos passageiros ao mesmo tempo, sem falha.
# Rodada 2: reservas cruzadas (mesmos trechos, ordens opostas): não pode travar.
# Rodada 3: carga + derruba a empresa B (dona de trechos) no meio.
# Rodada 4: carga + derruba a empresa A (coordenadora) no meio.
# Rodada 5: coordenador cai DEPOIS de gravar a decisão (falha simulada exata).
# Depois de cada rodada: religa, espera o prazo do provisório e confere as 3 invariantes.
set -u
PASSAGEIROS=${PASSAGEIROS:-90}
ESPERA=${ESPERA:-45}   # prazo do provisório (30s) + vigia (5s) + margem
CARGA="go run ./carga"
falhas=0

confere() {
  echo "-- esperando ${ESPERA}s para os provisórios se resolverem..."
  sleep "$ESPERA"
  if $CARGA -modo verificar; then echo ">> $1: OK"; else echo ">> $1: FALHOU"; falhas=$((falhas+1)); fi
}

echo "== preparando: 3 empresas limpas"
docker compose down -v >/dev/null 2>&1
docker compose up -d --build >/dev/null 2>&1 || { echo "docker compose falhou"; exit 1; }
sleep 3
$CARGA -modo semear

echo; echo "== rodada 1: $PASSAGEIROS passageiros simultâneos, sem falha"
$CARGA -modo carga -passageiros "$PASSAGEIROS" -data 2026-11-03
$CARGA -modo verificar && echo ">> rodada 1: OK" || falhas=$((falhas+1))

echo; echo "== rodada 2: reservas cruzadas (mesmos trechos de B e C, ordens opostas)"
if $CARGA -modo cruzado -rodadas 20 -data 2026-11-07 && $CARGA -modo verificar; then
  echo ">> reservas cruzadas: OK"
else
  echo ">> reservas cruzadas: FALHOU"; falhas=$((falhas+1))
fi

for empresa in b a; do
  data=2026-11-0$([ $empresa = b ] && echo 4 || echo 5)
  echo; echo "== rodada: carga + queda da empresa ${empresa^^} no meio (data $data)"
  $CARGA -modo semear -data "$data" >/dev/null
  $CARGA -modo carga -passageiros "$PASSAGEIROS" -data "$data" &
  sleep 0.6
  docker compose stop -t 0 "empresa-$empresa"
  wait
  sleep 5
  docker compose start "empresa-$empresa"
  confere "queda da empresa ${empresa^^}"
done

echo; echo "== rodada 5: coordenador A cai DEPOIS de gravar a decisão"
$CARGA -modo semear -data 2026-11-06 >/dev/null
curl -s -X POST "localhost:8081/simular-falha?ponto=apos-decisao"; echo
$CARGA -modo carga -passageiros 1 -data 2026-11-06
sleep 3
docker compose start empresa-a
confere "coordenador cai depois da decisao"

echo
if [ "$falhas" -eq 0 ]; then echo "RESULTADO: todas as rodadas OK"; else echo "RESULTADO: $falhas rodada(s) falharam"; exit 1; fi
