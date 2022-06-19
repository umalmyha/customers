FROM golang:1.18-alpine AS build

WORKDIR /app

COPY . .

RUN go mod download

RUN CGO_ENABLED=0 go build -o customers .


FROM alpine:latest

WORKDIR /app

COPY --from=build /app/customers /app

EXPOSE 3000

CMD ["/app/customers"]

