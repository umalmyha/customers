FROM golang:1.18-alpine AS build

WORKDIR /app

COPY . .

RUN apk update && apk add openssl
RUN openssl genpkey -algorithm ED25519 -outform pem -out private.ed
RUN openssl pkey -in private.ed -pubout > public.ed.pub

RUN go mod download
RUN CGO_ENABLED=0 go build -o customers .


FROM alpine:latest

WORKDIR /app

COPY --from=build /app/customers /app/private.ed /app/public.ed.pub /app/

EXPOSE 3000

CMD ["/app/customers"]

