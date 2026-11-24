package main

import (
	"errors"
	"net"
	"testing"
	"time"

	clientconfig "github.com/pescador95/memorydb/api/client/internal/config"
	memorydb "github.com/pescador95/memorydb/drivers/go"

	sharedconfig "github.com/pescador95/memorydb/shared/config"
)

type mockMemoryDBClient struct {
	pingErr   error
	setErr    error
	deleteErr error

	getResults []mockGetResult
	getCalls   int

	deleteResult bool

	pingCalls   int
	setCalls    int
	deleteCalls int
	closeCalls  int

	setOperationID memorydb.OperationID
	setKey         string
	setValue       []byte

	deleteOperationID memorydb.OperationID
	deleteKey         string
}

type mockGetResult struct {
	value []byte
	found bool
	err   error
}

func (m *mockMemoryDBClient) Ping() error {
	m.pingCalls++
	return m.pingErr
}

func (m *mockMemoryDBClient) Set(
	operationID memorydb.OperationID,
	key string,
	value []byte,
) error {
	m.setCalls++
	m.setOperationID = operationID
	m.setKey = key
	m.setValue = append([]byte(nil), value...)

	return m.setErr
}

func (m *mockMemoryDBClient) Get(key string) ([]byte, bool, error) {
	index := m.getCalls
	m.getCalls++

	if index >= len(m.getResults) {
		return nil, false, nil
	}

	result := m.getResults[index]

	return result.value, result.found, result.err
}

func (m *mockMemoryDBClient) Delete(
	operationID memorydb.OperationID,
	key string,
) (bool, error) {
	m.deleteCalls++
	m.deleteOperationID = operationID
	m.deleteKey = key

	return m.deleteResult, m.deleteErr
}

func (m *mockMemoryDBClient) Close() error {
	m.closeCalls++
	return nil
}

func TestExecuteExampleSuccess(t *testing.T) {
	client := &mockMemoryDBClient{
		getResults: []mockGetResult{
			{
				value: []byte("memorydb"),
				found: true,
			},
			{
				found: false,
			},
		},
		deleteResult: true,
	}

	var generatedIDs int

	newOperationID := func() (memorydb.OperationID, error) {
		generatedIDs++

		var id memorydb.OperationID
		id[0] = byte(generatedIDs)

		return id, nil
	}

	if err := executeExample(client, newOperationID); err != nil {
		t.Fatalf("executeExample retornou erro: %v", err)
	}

	if client.pingCalls != 1 {
		t.Errorf("chamadas PING: obtido %d, esperado 1", client.pingCalls)
	}

	if client.setCalls != 1 {
		t.Errorf("chamadas SET: obtido %d, esperado 1", client.setCalls)
	}

	if client.getCalls != 2 {
		t.Errorf("chamadas GET: obtido %d, esperado 2", client.getCalls)
	}

	if client.deleteCalls != 1 {
		t.Errorf("chamadas DELETE: obtido %d, esperado 1", client.deleteCalls)
	}

	if generatedIDs != 2 {
		t.Errorf("OperationIDs gerados: obtido %d, esperado 2", generatedIDs)
	}

	if client.setKey != "user:1" {
		t.Errorf("chave SET inesperada: %q", client.setKey)
	}

	if string(client.setValue) != "memorydb" {
		t.Errorf("valor SET inesperado: %q", client.setValue)
	}

	if client.setOperationID[0] != 1 {
		t.Errorf("OperationID do SET inesperado: %v", client.setOperationID)
	}

	if client.deleteOperationID[0] != 2 {
		t.Errorf("OperationID do DELETE inesperado: %v", client.deleteOperationID)
	}

	if client.deleteKey != "user:1" {
		t.Errorf("chave DELETE inesperada: %q", client.deleteKey)
	}
}

func TestExecuteExampleErrors(t *testing.T) {
	sentinel := errors.New("erro simulado")

	tests := []struct {
		name       string
		configure  func(*mockMemoryDBClient)
		failIDAt   int
		getResults []mockGetResult
		wantError  error
	}{
		{
			name: "ping failure",
			configure: func(client *mockMemoryDBClient) {
				client.pingErr = sentinel
			},
			wantError: sentinel,
		},
		{
			name:      "set operation id failure",
			failIDAt:  1,
			wantError: sentinel,
		},
		{
			name: "set failure",
			configure: func(client *mockMemoryDBClient) {
				client.setErr = sentinel
			},
			wantError: sentinel,
		},
		{
			name: "get failure",
			getResults: []mockGetResult{
				{err: sentinel},
			},
			wantError: sentinel,
		},
		{
			name: "key not found",
			getResults: []mockGetResult{
				{found: false},
			},
		},
		{
			name:     "delete operation id failure",
			failIDAt: 2,
			getResults: []mockGetResult{
				{value: []byte("memorydb"), found: true},
			},
			wantError: sentinel,
		},
		{
			name: "delete failure",
			configure: func(client *mockMemoryDBClient) {
				client.deleteErr = sentinel
			},
			getResults: []mockGetResult{
				{value: []byte("memorydb"), found: true},
			},
			wantError: sentinel,
		},
		{
			name: "get after delete failure",
			getResults: []mockGetResult{
				{value: []byte("memorydb"), found: true},
				{err: sentinel},
			},
			wantError: sentinel,
		},
		{
			name: "key still exists after delete",
			getResults: []mockGetResult{
				{value: []byte("memorydb"), found: true},
				{value: []byte("memorydb"), found: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockMemoryDBClient{
				getResults: tt.getResults,
			}

			if tt.configure != nil {
				tt.configure(client)
			}

			generatedIDs := 0

			newOperationID := func() (memorydb.OperationID, error) {
				generatedIDs++

				if generatedIDs == tt.failIDAt {
					return memorydb.OperationID{}, sentinel
				}

				return memorydb.OperationID{}, nil
			}

			err := executeExample(client, newOperationID)

			if tt.wantError != nil {
				if !errors.Is(err, tt.wantError) {
					t.Fatalf("esperava erro %v, recebido %v", tt.wantError, err)
				}
			} else if err == nil {
				t.Fatal("esperava erro, mas executeExample retornou nil")
			}
		})
	}
}

func TestRunWith(t *testing.T) {
	sentinel := errors.New("erro simulado")

	expectedConfig := clientconfig.Config{
		Address:     "127.0.0.1:6381",
		DialTimeout: 3 * time.Second,
	}

	tests := []struct {
		name          string
		loadErr       error
		dialErr       error
		pingErr       error
		wantError     error
		wantDialCalls int
		wantClose     int
	}{
		{
			name:      "configuration failure",
			loadErr:   sentinel,
			wantError: sentinel,
		},
		{
			name:          "dial failure",
			dialErr:       sentinel,
			wantError:     sentinel,
			wantDialCalls: 1,
		},
		{
			name:          "demonstration failure",
			pingErr:       sentinel,
			wantError:     sentinel,
			wantDialCalls: 1,
			wantClose:     1,
		},
		{
			name:          "success",
			wantDialCalls: 1,
			wantClose:     1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := &mockMemoryDBClient{
				pingErr: tt.pingErr,
				getResults: []mockGetResult{
					{value: []byte("memorydb"), found: true},
					{found: false},
				},
				deleteResult: true,
			}

			loadCalls := 0
			dialCalls := 0

			loadConfig := func() (clientconfig.Config, error) {
				loadCalls++
				return expectedConfig, tt.loadErr
			}

			dial := func(config memorydb.DialConfig) (memoryDBClient, error) {
				dialCalls++

				if config.Address != expectedConfig.Address {
					t.Errorf(
						"endereço recebido: %q, esperado %q",
						config.Address,
						expectedConfig.Address,
					)
				}

				if config.Timeout != expectedConfig.DialTimeout {
					t.Errorf(
						"timeout recebido: %s, esperado %s",
						config.Timeout,
						expectedConfig.DialTimeout,
					)
				}

				if tt.dialErr != nil {
					return nil, tt.dialErr
				}

				return client, nil
			}

			newOperationID := func() (memorydb.OperationID, error) {
				return memorydb.OperationID{}, nil
			}

			err := runWith(loadConfig, dial, newOperationID)

			if tt.wantError != nil {
				if !errors.Is(err, tt.wantError) {
					t.Fatalf("esperava erro %v, recebido %v", tt.wantError, err)
				}
			} else if err != nil {
				t.Fatalf("runWith retornou erro inesperado: %v", err)
			}

			if loadCalls != 1 {
				t.Errorf("chamadas de configuração: obtido %d, esperado 1", loadCalls)
			}

			if dialCalls != tt.wantDialCalls {
				t.Errorf("chamadas de conexão: obtido %d, esperado %d", dialCalls, tt.wantDialCalls)
			}

			if client.closeCalls != tt.wantClose {
				t.Errorf("chamadas Close: obtido %d, esperado %d", client.closeCalls, tt.wantClose)
			}
		})
	}
}

func TestRunRejectsInvalidConfiguration(t *testing.T) {
	t.Setenv(sharedconfig.EnvDialTimeout, "invalid")

	err := run()
	if err == nil {
		t.Fatal("esperava erro para configuração inválida")
	}
}

func TestDialClient(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("iniciar listener TCP: %v", err)
	}
	defer listener.Close()

	serverConnection := make(chan net.Conn, 1)

	go func() {
		conn, err := listener.Accept()
		if err == nil {
			serverConnection <- conn
		}
	}()

	client, err := dialClient(memorydb.DialConfig{
		Address: listener.Addr().String(),
		Timeout: time.Second,
	})
	if err != nil {
		t.Fatalf("dialClient retornou erro: %v", err)
	}

	defer client.Close()

	select {
	case conn := <-serverConnection:
		defer conn.Close()
	case <-time.After(time.Second):
		t.Fatal("servidor não aceitou a conexão")
	}
}
