FROM golang AS build
WORKDIR /src
COPY . .
ENV CGO_ENABLED=0
RUN go build -o /out/waveplus_prom waveplus_prom.go

FROM scratch AS bin
COPY --from=build /out/waveplus_prom /
CMD ["/waveplus_prom"]