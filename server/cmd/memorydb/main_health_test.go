package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"sync"
	"testing"
	"time"

	serverconfig "github.com/pescador95/memorydb/server/internal/config"
)

type createdListener struct {
	name string
	addr net.Addr
}

type acceptNotifyingListener struct {
	net.Listener
	accepted chan struct{}
	once     sync.Once
}

func (l *acceptNotifyingListener) Accept() (net.Conn, error) {
	conn, err := l.Listener.Accept()
	if err == nil {
		l.once.Do(func() {
			close(l.accepted)
		})
	}

	return conn, err
}

func TestRunWithHealthEndpointsDuringTCPShutdown(t *testing.T) {
	config := validServerConfig("127.0.0.1:0")
	config.HealthAddress = "127.0.0.1:0"
	config.ShutdownTimeout = 3 * time.Second

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	listenersCreated := make(chan createdListener, 2)
	tcpAccepted := make(chan struct{})
	done := make(chan error, 1)

	go func() {
		listenCalls := 0

		done <- runWith(
			func() (serverconfig.Config, error) {
				return config, nil
			},
			func(network, address string) (net.Listener, error) {
				listener, err := net.Listen(network, address)
				if err != nil {
					return nil, err
				}

				listenCalls++

				name := "TCP"
				if listenCalls == 1 {
					listener = &acceptNotifyingListener{
						Listener: listener,
						accepted: tcpAccepted,
					}
				} else {
					name = "health HTTP"
				}

				listenersCreated <- createdListener{
					name: name,
					addr: listener.Addr(),
				}

				return listener, nil
			},
			func(
				parent context.Context,
				signals ...os.Signal,
			) (context.Context, context.CancelFunc) {
				return ctx, func() {}
			},
		)
	}()

	var tcpAddress string
	var healthAddress string

	for range 2 {
		select {
		case created := <-listenersCreated:
			switch created.name {
			case "TCP":
				tcpAddress = created.addr.String()
			case "health HTTP":
				healthAddress = created.addr.String()
			default:
				t.Fatalf("unexpected listener name: %s", created.name)
			}

		case err := <-done:
			t.Fatalf("runWith terminou antes de criar os listeners: %v", err)

		case <-time.After(2 * time.Second):
			t.Fatal("timeout aguardando criação dos listeners")
		}
	}

	if tcpAddress == "" || healthAddress == "" {
		t.Fatalf(
			"endereços não inicializados: TCP=%q health=%q",
			tcpAddress,
			healthAddress,
		)
	}

	if tcpAddress == healthAddress {
		t.Fatalf("TCP e health deveriam usar portas distintas: %s", tcpAddress)
	}

	client := &http.Client{
		Timeout: 500 * time.Millisecond,
	}

	liveURL := fmt.Sprintf("http://%s/livez", healthAddress)
	readyURL := fmt.Sprintf("http://%s/readyz", healthAddress)

	waitForHTTPStatus(t, client, liveURL, http.StatusOK)
	waitForHTTPStatus(t, client, readyURL, http.StatusOK)

	// Mantém uma conexão TCP ativa com um cabeçalho de frame incompleto.
	// O handler aguarda os bytes restantes enquanto o shutdown inicia.
	tcpConn, err := net.DialTimeout("tcp", tcpAddress, time.Second)
	if err != nil {
		t.Fatalf("conectar ao servidor TCP: %v", err)
	}
	defer tcpConn.Close()

	if _, err := tcpConn.Write([]byte{0}); err != nil {
		t.Fatalf("enviar parte do cabeçalho TCP: %v", err)
	}

	// Aguarda o Accept do servidor antes de iniciar o shutdown.
	select {
	case <-tcpAccepted:
	case <-time.After(2 * time.Second):
		t.Fatal("servidor TCP não aceitou a conexão")
	}

	// Dá oportunidade ao Serve de registrar a conexão e iniciar o handler.
	time.Sleep(50 * time.Millisecond)

	cancel()

	// O TCP entra em STOPPING antes de iniciar a drenagem. O HTTP permanece
	// ativo durante esse intervalo para que o orquestrador observe readiness 503.
	waitForHTTPStatus(t, client, readyURL, http.StatusServiceUnavailable)
	waitForHTTPStatus(t, client, liveURL, http.StatusOK)

	// Libera o handler para que o shutdown TCP termine sem esgotar o prazo.
	if err := tcpConn.Close(); err != nil {
		t.Errorf("fechar conexão TCP: %v", err)
	}

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("runWith retornou erro durante shutdown: %v", err)
		}

	case <-time.After(4 * time.Second):
		t.Fatal("timeout aguardando encerramento dos servidores")
	}
}

func waitForHTTPStatus(
	t *testing.T,
	client *http.Client,
	url string,
	expectedStatus int,
) {
	t.Helper()

	deadline := time.Now().Add(2 * time.Second)
	var lastStatus int
	var lastErr error

	for time.Now().Before(deadline) {
		response, err := client.Get(url)
		if err != nil {
			lastErr = err
		} else {
			lastStatus = response.StatusCode
			lastErr = nil
			_ = response.Body.Close()

			if lastStatus == expectedStatus {
				return
			}
		}

		time.Sleep(10 * time.Millisecond)
	}

	if lastErr != nil {
		t.Fatalf(
			"GET %s: esperado status %d; último erro: %v",
			url,
			expectedStatus,
			lastErr,
		)
	}

	t.Fatalf(
		"GET %s: status recebido %d; esperado %d",
		url,
		lastStatus,
		expectedStatus,
	)
}
