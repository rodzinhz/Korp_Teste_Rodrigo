package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

// ---------- Modelos ----------

type Produto struct {
	ID        int     `json:"id"`
	Codigo    string  `json:"codigo"`
	Descricao string  `json:"descricao"`
	Categoria string  `json:"categoria,omitempty"`
	Preco     float64 `json:"preco"`
	Saldo     int     `json:"saldo"`
}

// ErroAPI é o formato padrão de erro devolvido por todos os endpoints.
// Padronizar isso permite que o frontend leia err.error.erro em vez
// de depender de texto puro (http.Error tradicional).
type ErroAPI struct {
	Erro string `json:"erro"`
}

var db *sql.DB

// ---------- Infra ----------

func iniciarBanco() {
	var err error

	db, err = sql.Open("sqlite3", "./estoque.db")
	if err != nil {
		log.Fatal("Erro ao abrir banco:", err)
	}

	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS produtos (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		codigo TEXT NOT NULL,
		descricao TEXT NOT NULL,
		categoria TEXT NOT NULL DEFAULT '',
		preco REAL NOT NULL,
		saldo INTEGER NOT NULL
	)`)
	if err != nil {
		log.Fatal("Erro ao criar tabela:", err)
	}

	// Migração leve: se o banco já existia sem a coluna categoria, adiciona.
	db.Exec(`ALTER TABLE produtos ADD COLUMN categoria TEXT NOT NULL DEFAULT ''`)

	log.Println("Banco de dados iniciado!")
}

func responderErro(w http.ResponseWriter, status int, mensagem string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(ErroAPI{Erro: mensagem})
}

func responderJSON(w http.ResponseWriter, status int, payload interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(payload)
}

func habilitarCORS(w http.ResponseWriter) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Idempotency-Key")
}

// ---------- Handlers de Produto ----------

func listarProdutos(w http.ResponseWriter, r *http.Request) {
	rows, err := db.Query("SELECT id, codigo, descricao, categoria, preco, saldo FROM produtos")
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Erro ao buscar produtos")
		return
	}
	defer rows.Close()

	produtos := []Produto{}
	for rows.Next() {
		var p Produto
		if err := rows.Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Categoria, &p.Preco, &p.Saldo); err != nil {
			responderErro(w, http.StatusInternalServerError, "Erro ao ler produto")
			return
		}
		produtos = append(produtos, p)
	}

	responderJSON(w, http.StatusOK, produtos)
}

func criarProduto(w http.ResponseWriter, r *http.Request) {
	var p Produto

	if err := json.NewDecoder(r.Body).Decode(&p); err != nil {
		responderErro(w, http.StatusBadRequest, "Erro ao processar os dados do produto")
		return
	}

	if p.Codigo == "" || p.Descricao == "" {
		responderErro(w, http.StatusBadRequest, "Código e descrição são obrigatórios")
		return
	}
	if p.Saldo < 0 {
		responderErro(w, http.StatusBadRequest, "Saldo não pode ser negativo")
		return
	}

	result, err := db.Exec(
		"INSERT INTO produtos (codigo, descricao, categoria, preco, saldo) VALUES (?, ?, ?, ?, ?)",
		p.Codigo, p.Descricao, p.Categoria, p.Preco, p.Saldo,
	)
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Erro ao salvar produto")
		log.Println("Erro ao inserir produto:", err)
		return
	}

	id, _ := result.LastInsertId()
	p.ID = int(id)

	responderJSON(w, http.StatusCreated, p)
}

// descontarSaldo diminui o saldo de forma segura para concorrência.
// Em vez de "ler saldo -> checar em Go -> gravar", o UPDATE já embute a
// condição (saldo >= quantidade) no próprio SQL. Assim, se duas notas
// tentarem descontar o último item ao mesmo tempo, o SQLite serializa as
// escritas e só uma das duas consegue: a segunda recebe RowsAffected = 0
// e devolvemos "saldo insuficiente", sem sobrescrever o resultado da outra.
func descontarSaldo(w http.ResponseWriter, r *http.Request) {
	var desconto struct {
		ProdutoID  int `json:"produto_id"`
		Quantidade int `json:"quantidade"`
	}

	if err := json.NewDecoder(r.Body).Decode(&desconto); err != nil {
		responderErro(w, http.StatusBadRequest, "Erro ao processar desconto")
		return
	}
	if desconto.Quantidade <= 0 {
		responderErro(w, http.StatusBadRequest, "Quantidade inválida")
		return
	}

	result, err := db.Exec(
		"UPDATE produtos SET saldo = saldo - ? WHERE id = ? AND saldo >= ?",
		desconto.Quantidade, desconto.ProdutoID, desconto.Quantidade,
	)
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Erro ao atualizar saldo")
		return
	}

	linhas, _ := result.RowsAffected()
	if linhas == 0 {
		responderErro(w, http.StatusConflict, "Saldo insuficiente ou produto não encontrado")
		return
	}

	var p Produto
	err = db.QueryRow(
		"SELECT id, codigo, descricao, categoria, preco, saldo FROM produtos WHERE id = ?",
		desconto.ProdutoID,
	).Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Categoria, &p.Preco, &p.Saldo)
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Erro ao ler produto atualizado")
		return
	}

	responderJSON(w, http.StatusOK, p)
}

func devolverSaldo(w http.ResponseWriter, r *http.Request) {
	var desconto struct {
		ProdutoID  int `json:"produto_id"`
		Quantidade int `json:"quantidade"`
	}

	if err := json.NewDecoder(r.Body).Decode(&desconto); err != nil {
		responderErro(w, http.StatusBadRequest, "Erro ao processar devolução")
		return
	}

	_, err := db.Exec(
		"UPDATE produtos SET saldo = saldo + ? WHERE id = ?",
		desconto.Quantidade, desconto.ProdutoID,
	)
	if err != nil {
		responderErro(w, http.StatusInternalServerError, "Erro ao devolver saldo")
		return
	}

	w.WriteHeader(http.StatusOK)
}

func excluirProduto(w http.ResponseWriter, r *http.Request) {
	idStr := r.URL.Query().Get("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		responderErro(w, http.StatusBadRequest, "ID inválido")
		return
	}
	db.Exec("DELETE FROM produtos WHERE id = ?", id)
	w.WriteHeader(http.StatusNoContent)
}

func buscarProdutoPorID(w http.ResponseWriter, r *http.Request) {
	idStr := strings.TrimPrefix(r.URL.Path, "/produtos/")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		responderErro(w, http.StatusBadRequest, "ID inválido")
		return
	}

	var p Produto
	err = db.QueryRow("SELECT id, codigo, descricao, categoria, preco, saldo FROM produtos WHERE id = ?", id).
		Scan(&p.ID, &p.Codigo, &p.Descricao, &p.Categoria, &p.Preco, &p.Saldo)

	if err != nil {
		responderErro(w, http.StatusNotFound, "Produto não encontrado")
		return
	}

	responderJSON(w, http.StatusOK, p)
}

// ---------- IA: sugestão de descrição/categoria ----------
// Requisito opcional "Uso de Inteligência Artificial". O candidato digita
// algo curto (ex: "parafuso 6mm") e a API da Anthropic devolve uma
// descrição comercial e uma categoria sugerida, em JSON estruturado.
// Chave lida de variável de ambiente para não expor segredo no código.

type sugestaoRequest struct {
	Texto string `json:"texto"`
}

type sugestaoResponse struct {
	Descricao string `json:"descricao"`
	Categoria string `json:"categoria"`
}

// ==== Structs do Gemini ====

type geminiRequestBody struct {
	Contents []geminiContent `json:"contents"`
}

type geminiContent struct {
	Parts []geminiPart `json:"parts"`
}

type geminiPart struct {
	Text string `json:"text"`
}

type geminiResponseBody struct {
	Candidates []geminiCandidate `json:"candidates"`
}

type geminiCandidate struct {
	Content geminiContent `json:"content"`
}

// ==== Handler ====

func sugerirDescricao(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	var req sugestaoRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Texto == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{"erro": "texto é obrigatório"})
		return
	}

	apiKey := os.Getenv("GEMINI_API_KEY")
	if apiKey == "" {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"erro": "GEMINI_API_KEY não configurada"})
		return
	}

	prompt := fmt.Sprintf(
		`Com base no texto a seguir, sugira uma descrição curta de produto e uma categoria. `+
			`Responda SOMENTE em JSON no formato {"descricao": "...", "categoria": "..."}, sem markdown, sem explicação. Texto: %s`,
		req.Texto,
	)

	geminiReq := geminiRequestBody{
		Contents: []geminiContent{
			{Parts: []geminiPart{{Text: prompt}}},
		},
	}

	body, err := json.Marshal(geminiReq)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"erro": "falha ao montar requisição"})
		return
	}

	url := fmt.Sprintf(
		"https://generativelanguage.googleapis.com/v1beta/models/gemini-3.6-flash:generateContent?key=%s",
		apiKey,
	)

	client := &http.Client{Timeout: 15 * time.Second}
	httpReq, err := http.NewRequest(http.MethodPost, url, bytes.NewBuffer(body))
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"erro": "falha ao criar requisição"})
		return
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(httpReq)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"erro": "falha ao chamar API de IA"})
		return
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"erro": "falha ao ler resposta da IA"})
		return
	}

	if resp.StatusCode != http.StatusOK {
		log.Printf("Gemini retornou status %d: %s\n", resp.StatusCode, string(respBody))
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]string{"erro": "API de IA retornou erro"})
		return
	}

	var geminiResp geminiResponseBody
	if err := json.Unmarshal(respBody, &geminiResp); err != nil ||
		len(geminiResp.Candidates) == 0 ||
		len(geminiResp.Candidates[0].Content.Parts) == 0 {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"erro": "resposta inesperada da IA"})
		return
	}

	textoResposta := geminiResp.Candidates[0].Content.Parts[0].Text

	// O Gemini às vezes envolve o JSON em ```json ... ``` — limpamos antes de parsear
	textoResposta = strings.TrimSpace(textoResposta)
	textoResposta = strings.TrimPrefix(textoResposta, "```json")
	textoResposta = strings.TrimPrefix(textoResposta, "```")
	textoResposta = strings.TrimSuffix(textoResposta, "```")
	textoResposta = strings.TrimSpace(textoResposta)

	var sugestao sugestaoResponse
	if err := json.Unmarshal([]byte(textoResposta), &sugestao); err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		json.NewEncoder(w).Encode(map[string]string{"erro": "IA não retornou JSON válido"})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(sugestao)
}

// ---------- main ----------

func main() {
	iniciarBanco()

	http.HandleFunc("/produtos", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		switch r.Method {
		case "GET":
			listarProdutos(w, r)
		case "POST":
			criarProduto(w, r)
		case "DELETE":
			excluirProduto(w, r)
		default:
			responderErro(w, http.StatusMethodNotAllowed, "Método não permitido")
		}
	})

	http.HandleFunc("/produtos/", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "GET" {
			buscarProdutoPorID(w, r)
		} else {
			responderErro(w, http.StatusMethodNotAllowed, "Método não permitido")
		}
	})

	http.HandleFunc("/produtos/descontar", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" {
			descontarSaldo(w, r)
		} else {
			responderErro(w, http.StatusMethodNotAllowed, "Método não permitido")
		}
	})

	http.HandleFunc("/produtos/devolver", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" {
			devolverSaldo(w, r)
		} else {
			responderErro(w, http.StatusMethodNotAllowed, "Método não permitido")
		}
	})

	http.HandleFunc("/produtos/sugerir-descricao", func(w http.ResponseWriter, r *http.Request) {
		habilitarCORS(w)
		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.Method == "POST" {
			sugerirDescricao(w, r)
		} else {
			responderErro(w, http.StatusMethodNotAllowed, "Método não permitido")
		}
	})

	log.Println("Estoque service rodando na porta 8081...")
	log.Fatal(http.ListenAndServe(":8081", nil))
}
