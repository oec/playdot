package main

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"flag"
	"html/template"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
)

var (
	tls     = flag.String("t", ":8443", "[ip]:port to tls-listen to")
	nontls  = flag.String("l", "", "optional, non-tls [ip]:port to listen to")
	config  = flag.String("cfg", "tools.json", "config file with tool-definitions")
	cert    = flag.String("cert", "cert.pem", "certitifate")
	key     = flag.String("key", "key.pem", "key")
	savedir = flag.String("d", "saved", "direcotry to save the pics.")
)

type Tool struct {
	Name          string
	Cmd           string
	Args          []string
	NeedsFile     bool
	EncodeBase64  bool
	Suffix        string
	Description   string
	Documentation map[string]string
	Example       string
	BgColor       string
}

func (t *Tool) execute(in io.Reader, w http.ResponseWriter) {
	var args []string

	if t.NeedsFile {
		tmpf, e := ioutil.TempFile(".", "*"+t.Suffix)
		if e != nil {
			log.Printf("couldn't create tmp-file: %v\n", e)
			http.Error(w, e.Error(), http.StatusInternalServerError)
			return
		} else if _, e = io.Copy(tmpf, in); e != nil {
			log.Printf("couldn't write to tmp-file: %v\n", e)
			http.Error(w, e.Error(), http.StatusInternalServerError)
			return
		}
		defer os.Remove(tmpf.Name())
		// log.Printf("using tempfile: %q\n", tmpf.Name())

		args = []string{}
		args = append(args, t.Args...)
		args = append(args, tmpf.Name())
	} else {
		args = t.Args
	}

	var (
		cmd      = exec.Command(t.Cmd, args...)
		err, buf = &bytes.Buffer{}, &bytes.Buffer{}
	)

	if !t.NeedsFile {
		cmd.Stdin = in
	}
	cmd.Stderr = err
	cmd.Stdout = buf

	if e := cmd.Run(); e == nil {
		if t.EncodeBase64 {
			w.Header().Add("Content-Type", "img/png")
			io.Copy(base64.NewEncoder(base64.StdEncoding, w), buf)
		} else {
			io.Copy(w, buf)
		}
	} else {
		log.Printf("%s returned error\n", t.Name)
		http.Error(w, err.String(), http.StatusBadRequest)
	}
}

func (t *Tool) compile() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		t.execute(r.Body, w)
	}
}

func (t *Tool) svg() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		if file, err := os.Open(filepath.Join(*savedir, name+t.Suffix)); err != nil {
			log.Println(err)
			http.Error(w, "couldn't open file", http.StatusBadRequest)
		} else {
			defer file.Close()
			t.execute(file, w)
		}
	}
}

const maxSnippetSize = 64 * 1024

func (t *Tool) save() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		body, err := ioutil.ReadAll(io.LimitReader(r.Body, maxSnippetSize))
		if err != nil {
			log.Println(err)
			http.Error(w, "body too large", http.StatusBadRequest)
			return
		}
		r.Body.Close()

		// Create a filename taking the first 10 characters of the sha1-sum of
		// content.  Based on code from the golang-playground.
		h := sha1.New()
		io.Copy(h, bytes.NewBuffer(body))
		sum := h.Sum(nil)
		b := make([]byte, base64.URLEncoding.EncodedLen(len(sum)))
		base64.URLEncoding.Encode(b, sum)
		name := string(b)[:10]

		// Write snippet to file.  TODO: Shall we return error if file exists?
		if file, err := os.Create(filepath.Join(*savedir, name+t.Suffix)); err != nil {
			log.Println(err)
			http.Error(w, "couldn't create file", http.StatusInternalServerError)
		} else if err = file.Chmod(0644); err != nil {
			log.Println(err)
			http.Error(w, "couldn't setup file", http.StatusInternalServerError)
		} else if _, err = io.Copy(file, bytes.NewBuffer(body)); err != nil {
			log.Println(err)
			http.Error(w, "couldn't write file", http.StatusInternalServerError)
		} else if err = file.Close(); err != nil {
			log.Println(err)
			http.Error(w, "couldn't close file", http.StatusInternalServerError)
		}
		w.Write([]byte(name))
	}
}

func (t *Tool) load() func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		name := filepath.Base(r.URL.Path)
		if file, err := os.Open(filepath.Join(*savedir, name+t.Suffix)); err != nil {
			log.Println(err)
			http.Error(w, "couldn't open file", http.StatusBadRequest)
		} else {
			defer file.Close()
			io.Copy(w, file)
		}
	}
}

func (t *Tool) index(tmpl *template.Template, data interface{}) func(http.ResponseWriter, *http.Request) {
	return func(w http.ResponseWriter, r *http.Request) {
		err := tmpl.ExecuteTemplate(w, "index.html", map[string]interface{}{"Tools": data, "Cur": t})
		if err != nil {
			log.Printf("error executing template for %s: %v", t.Name, err)
		}
	}
}

func main() {
	var tools = []*Tool{}
	var tmpl *template.Template

	flag.Parse()
	if cfg, err := os.Open(*config); err != nil {
		log.Fatal(err)
	} else if err = json.NewDecoder(cfg).Decode(&tools); err != nil {
		log.Fatalf("error loading %s: %v\n", *config, err)
	} else if tmpl, err = template.ParseFiles("index.html"); err != nil {
		log.Fatalf("error parsing index.html: %v", err)
	}

	for _, tool := range tools {
		pre := "/" + tool.Name + "/"
		http.HandleFunc(pre+"c", tool.compile())
		http.HandleFunc(pre+"s", tool.save())
		http.HandleFunc(pre+"l/", tool.load())
		http.HandleFunc(pre+"svg/", tool.svg())
		http.HandleFunc(pre, tool.index(tmpl, tools))
		log.Println("handler for", pre, "registered")
	}
	http.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("static"))))
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/"+tools[0].Name, http.StatusFound)
	})

	if len(*nontls) > 0 {
		log.Println("listening non-tls on", *nontls)
		log.Fatal(http.ListenAndServe(*nontls, nil))
	}
	log.Println("listening tls on", *tls)
	log.Fatal(http.ListenAndServeTLS(*tls, *cert, *key, nil))
}
