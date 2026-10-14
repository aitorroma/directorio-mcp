// comprobar valida las fichas de servidores/ y, con -probar, se conecta a
// cada servidor MCP para listar sus herramientas. Con -indice escribe el
// índice que se publica (index.json).
//
//	go run ./cmd/comprobar                      # solo valida las fichas
//	go run ./cmd/comprobar -probar              # valida y prueba los servidores
//	go run ./cmd/comprobar -probar -indice publicar/index.json
//	go run ./cmd/comprobar -probar servidores/uno.json   # solo esos
package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

// Ficha es lo que se escribe a mano en servidores/<id>.json.
type Ficha struct {
	Nombre        string   `json:"nombre"`
	URL           string   `json:"url"`
	Descripcion   string   `json:"descripcion"`
	Categoria     string   `json:"categoria"`
	Etiquetas     []string `json:"etiquetas,omitempty"`
	Autenticacion string   `json:"autenticacion"`      // ninguna, cabecera, oauth
	Cabecera      string   `json:"cabecera,omitempty"` // con «cabecera»: su nombre (Authorization, X-API-Key…)
	ComoClave     string   `json:"como_conseguir_clave,omitempty"`
	Web           string   `json:"web,omitempty"`
	Idiomas       []string `json:"idiomas,omitempty"`
	Pais          string   `json:"pais,omitempty"`
}

// Herramienta es lo que se ve de cada herramienta en el índice.
type Herramienta struct {
	Nombre      string `json:"nombre"`
	Descripcion string `json:"descripcion,omitempty"`
	SoloLectura bool   `json:"solo_lectura"`
}

// Entrada es una ficha con lo comprobado.
type Entrada struct {
	ID string `json:"id"`
	Ficha
	Logo         string        `json:"logo,omitempty"` // logos/<id>.svg o .png, junto al índice
	Estado       string        `json:"estado"`         // ok, necesita_clave, caido, sin_probar
	Error        string        `json:"error,omitempty"`
	Servidor     string        `json:"servidor,omitempty"` // serverInfo.title o name
	Herramientas []Herramienta `json:"herramientas,omitempty"`
	SoloLectura  bool          `json:"solo_lectura"` // todas sus herramientas dicen que solo leen
	Comprobado   time.Time     `json:"comprobado"`
}

type Indice struct {
	Version    int       `json:"version"`
	Generado   time.Time `json:"generado"`
	Categorias []string  `json:"categorias"`
	Servidores []Entrada `json:"servidores"`
}

var Categorias = []string{"motor", "viajes", "compras", "finanzas", "salud", "hogar", "trabajo", "desarrollo", "informacion", "ocio", "administracion", "otros"}

var reID = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{1,48}$`)

func main() {
	probar := flag.Bool("probar", false, "conectarse a cada servidor y listar sus herramientas")
	indice := flag.String("indice", "", "escribir el índice en este fichero")
	flag.Parse()
	files := flag.Args()
	if len(files) == 0 {
		files, _ = filepath.Glob("servidores/*.json")
	}
	sort.Strings(files)
	var (
		entradas []Entrada
		fallos   []string
		urls     = map[string]string{}
	)
	for _, f := range files {
		e, err := leer(f)
		if err != nil {
			fallos = append(fallos, fmt.Sprintf("%s: %v", f, err))
			continue
		}
		if otro, ok := urls[e.URL]; ok {
			fallos = append(fallos, fmt.Sprintf("%s: la misma URL que %s", f, otro))
			continue
		}
		urls[e.URL] = f
		entradas = append(entradas, e)
	}
	if *probar {
		var wg sync.WaitGroup
		sem := make(chan struct{}, 6)
		for i := range entradas {
			wg.Add(1)
			go func(e *Entrada) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				comprobar(e)
			}(&entradas[i])
		}
		wg.Wait()
	}
	for _, e := range entradas {
		linea := fmt.Sprintf("%-28s %-15s", e.ID, e.Estado)
		if len(e.Herramientas) > 0 {
			linea += fmt.Sprintf(" %d herramientas", len(e.Herramientas))
			if e.SoloLectura {
				linea += ", todas de solo lectura"
			}
		}
		if e.Error != "" {
			linea += " — " + e.Error
		}
		fmt.Println(linea)
		// En un pull request, un servidor sin clave que no responde no entra.
		if *probar && e.Estado == "caido" && e.Autenticacion == "ninguna" && len(flag.Args()) > 0 {
			fallos = append(fallos, fmt.Sprintf("%s: el servidor no responde como MCP: %s", e.ID, e.Error))
		}
	}
	if *indice != "" {
		ix := Indice{Version: 1, Generado: time.Now().UTC(), Categorias: Categorias, Servidores: entradas}
		raw, _ := json.MarshalIndent(ix, "", "  ")
		os.MkdirAll(filepath.Dir(*indice), 0o755)
		if err := os.WriteFile(*indice, append(raw, '\n'), 0o644); err != nil {
			fallos = append(fallos, err.Error())
		}
	}
	if len(fallos) > 0 {
		fmt.Fprintln(os.Stderr, "\nProblemas:")
		for _, f := range fallos {
			fmt.Fprintln(os.Stderr, "  - "+f)
		}
		os.Exit(1)
	}
}

func leer(f string) (Entrada, error) {
	id := strings.TrimSuffix(filepath.Base(f), ".json")
	if !reID.MatchString(id) {
		return Entrada{}, fmt.Errorf("el nombre del fichero va en minúsculas, números y guiones (%q)", id)
	}
	raw, err := os.ReadFile(f)
	if err != nil {
		return Entrada{}, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var fi Ficha
	if err := dec.Decode(&fi); err != nil {
		return Entrada{}, fmt.Errorf("JSON no válido: %v", err)
	}
	var errs []string
	falta := func(cond bool, msg string) {
		if cond {
			errs = append(errs, msg)
		}
	}
	u, err := url.Parse(fi.URL)
	falta(strings.TrimSpace(fi.Nombre) == "" || len(fi.Nombre) > 60, "nombre: obligatorio, como mucho 60 caracteres")
	falta(err != nil || u.Scheme != "https" || u.Host == "", "url: tiene que ser https://")
	falta(len(fi.Descripcion) < 30 || len(fi.Descripcion) > 400, "descripcion: entre 30 y 400 caracteres, en español")
	falta(!contiene(Categorias, fi.Categoria), "categoria: una de "+strings.Join(Categorias, ", "))
	falta(!contiene([]string{"ninguna", "cabecera", "oauth"}, fi.Autenticacion), "autenticacion: ninguna, cabecera u oauth")
	falta(fi.Autenticacion == "cabecera" && fi.Cabecera == "", "cabecera: con autenticacion «cabecera», el nombre de la cabecera (Authorization, X-API-Key…)")
	falta(fi.Web != "" && !strings.HasPrefix(fi.Web, "https://"), "web: tiene que ser https://")
	falta(len(fi.Etiquetas) > 10, "etiquetas: como mucho 10")
	falta(fi.Pais != "" && len(fi.Pais) != 2, "pais: código de dos letras (ES)")
	if len(errs) > 0 {
		return Entrada{}, errors.New(strings.Join(errs, "; "))
	}
	logo, err := buscarLogo(filepath.Dir(filepath.Dir(f)), id)
	if err != nil {
		return Entrada{}, err
	}
	return Entrada{ID: id, Ficha: fi, Estado: "sin_probar", Logo: logo}, nil
}

// Lo que no puede llevar un logo SVG: nada que se ejecute ni que cargue algo
// de fuera.
var reSVGMalo = regexp.MustCompile(`(?i)<script|<foreignobject|<iframe|<image|\bon[a-z]+\s*=|javascript:|(xlink:)?href\s*=\s*["'][^#"']|url\(\s*["']?[^#)"' ]`)

// buscarLogo comprueba logos/<id>.svg o .png, si hay.
func buscarLogo(raiz, id string) (string, error) {
	for _, ext := range []string{".svg", ".png"} {
		rel := "logos/" + id + ext
		raw, err := os.ReadFile(filepath.Join(raiz, rel))
		if err != nil {
			continue
		}
		switch ext {
		case ".svg":
			if len(raw) > 50<<10 {
				return "", fmt.Errorf("%s: como mucho 50 KB", rel)
			}
			if !bytes.Contains(raw, []byte("<svg")) {
				return "", fmt.Errorf("%s: no es un SVG", rel)
			}
			if m := reSVGMalo.Find(raw); m != nil {
				return "", fmt.Errorf("%s: no puede llevar scripts, eventos ni enlaces externos (%q)", rel, m)
			}
		case ".png":
			if len(raw) > 100<<10 {
				return "", fmt.Errorf("%s: como mucho 100 KB", rel)
			}
			if !bytes.HasPrefix(raw, []byte("\x89PNG\r\n\x1a\n")) {
				return "", fmt.Errorf("%s: no es un PNG", rel)
			}
		}
		return rel, nil
	}
	return "", nil
}

func contiene(l []string, s string) bool {
	for _, x := range l {
		if x == s {
			return true
		}
	}
	return false
}

// ---- probar un servidor MCP (HTTP «streamable») ----

var errClave = errors.New("pide autenticación")

type cliente struct {
	url, sesion string
	http        *http.Client
	id          int
}

func comprobar(e *Entrada) {
	e.Comprobado = time.Now().UTC()
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	c := &cliente{url: e.URL, http: &http.Client{Timeout: 30 * time.Second}}
	var ini struct {
		ServerInfo struct{ Name, Title string } `json:"serverInfo"`
	}
	err := c.llamar(ctx, "initialize", map[string]any{"protocolVersion": "2025-06-18", "capabilities": map[string]any{},
		"clientInfo": map[string]any{"name": "directorio-mcp", "version": "1"}}, &ini)
	if err == nil {
		e.Servidor = ini.ServerInfo.Title
		if e.Servidor == "" {
			e.Servidor = ini.ServerInfo.Name
		}
		c.avisar(ctx, "notifications/initialized")
		e.Herramientas, err = c.herramientas(ctx)
	}
	switch {
	case errors.Is(err, errClave):
		e.Estado = "necesita_clave"
		if e.Autenticacion == "ninguna" {
			e.Estado, e.Error = "caido", "pide autenticación pero la ficha dice que no"
		}
	case err != nil:
		e.Estado, e.Error = "caido", err.Error()
	default:
		e.Estado = "ok"
		e.SoloLectura = len(e.Herramientas) > 0
		for _, h := range e.Herramientas {
			e.SoloLectura = e.SoloLectura && h.SoloLectura
		}
	}
}

func (c *cliente) herramientas(ctx context.Context) ([]Herramienta, error) {
	var out []Herramienta
	cursor := ""
	for pagina := 0; pagina < 20; pagina++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var res struct {
			Tools []struct {
				Name, Title, Description string
				Annotations              struct {
					ReadOnlyHint bool `json:"readOnlyHint"`
				} `json:"annotations"`
			} `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := c.llamar(ctx, "tools/list", params, &res); err != nil {
			return nil, err
		}
		for _, t := range res.Tools {
			d := strings.TrimSpace(t.Description)
			if i := strings.IndexAny(d, "\n"); i > 0 {
				d = d[:i]
			}
			if len(d) > 200 {
				d = d[:200] + "…"
			}
			out = append(out, Herramienta{Nombre: t.Name, Descripcion: d, SoloLectura: t.Annotations.ReadOnlyHint})
		}
		if cursor = res.NextCursor; cursor == "" {
			break
		}
	}
	return out, nil
}

func (c *cliente) enviar(ctx context.Context, cuerpo map[string]any) (*http.Response, error) {
	raw, _ := json.Marshal(cuerpo)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url, bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", "2025-06-18")
	req.Header.Set("User-Agent", "directorio-mcp/1 (+https://github.com/aitorroma/directorio-mcp)")
	if c.sesion != "" {
		req.Header.Set("Mcp-Session-Id", c.sesion)
	}
	return c.http.Do(req)
}

func (c *cliente) avisar(ctx context.Context, metodo string) {
	if res, err := c.enviar(ctx, map[string]any{"jsonrpc": "2.0", "method": metodo}); err == nil {
		io.Copy(io.Discard, res.Body)
		res.Body.Close()
	}
}

func (c *cliente) llamar(ctx context.Context, metodo string, params any, out any) error {
	c.id++
	res, err := c.enviar(ctx, map[string]any{"jsonrpc": "2.0", "id": c.id, "method": metodo, "params": params})
	if err != nil {
		return fmt.Errorf("no responde: %v", err)
	}
	defer res.Body.Close()
	if s := res.Header.Get("Mcp-Session-Id"); s != "" {
		c.sesion = s
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized || res.StatusCode == http.StatusForbidden:
		return errClave
	case res.StatusCode != http.StatusOK:
		return fmt.Errorf("%s: HTTP %d", metodo, res.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(res.Body, 8<<20))
	if err != nil {
		return err
	}
	// La respuesta llega como JSON o como eventos (text/event-stream).
	if strings.HasPrefix(res.Header.Get("Content-Type"), "text/event-stream") {
		var data []byte
		sc := bufio.NewScanner(bytes.NewReader(body))
		sc.Buffer(make([]byte, 1<<20), 8<<20)
		for sc.Scan() {
			if d, ok := strings.CutPrefix(sc.Text(), "data:"); ok && strings.Contains(d, `"id"`) {
				data = []byte(strings.TrimSpace(d))
			}
		}
		body = data
	}
	var r struct {
		Result json.RawMessage `json:"result"`
		Error  *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return fmt.Errorf("%s: no contesta en JSON-RPC", metodo)
	}
	if r.Error != nil {
		return fmt.Errorf("%s: %s", metodo, r.Error.Message)
	}
	return json.Unmarshal(r.Result, out)
}
