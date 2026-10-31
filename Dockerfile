# Imagem única do VaiJunto: serve para QUALQUER empresa. Quem diz qual empresa
# o contêiner é são as variáveis de ambiente (EMPRESA, PARES...).
#
# Build em dois estágios: o primeiro tem o compilador Go e gera os binários;
# o segundo copia só os binários para uma imagem mínima.

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod ./
COPY protocolo ./protocolo
COPY servidor ./servidor
COPY cliente ./cliente
COPY carga ./carga
RUN CGO_ENABLED=0 go build -o /bin/servidor ./servidor && \
    CGO_ENABLED=0 go build -o /bin/cliente ./cliente && \
    CGO_ENABLED=0 go build -o /bin/carga ./carga

FROM alpine:3.20
COPY --from=build /bin/servidor /bin/cliente /bin/carga /bin/
RUN mkdir -p /dados
ENV DADOS=/dados/estado.json
# 8080 = API REST entre servidores; 9000 = clientes TCP
EXPOSE 8080 9000
CMD ["/bin/servidor"]
