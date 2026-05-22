package llm

import (
	"bytes"
	"fmt"
	"io/fs"
	"text/template"
)

// PromptVars are the variables every prompt template receives.
type PromptVars struct {
	Kind       string
	Topic      string
	Difficulty int
	Count      int
}

// LoadPrompts parses every `*.tmpl` file under the prompts/ FS into a
// single template set; LookupKind() returns the rendered text for a kind.
type Prompts struct {
	t *template.Template
}

func LoadPrompts(fsys fs.FS) (*Prompts, error) {
	t := template.New("prompts")
	err := fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || len(path) < 5 || path[len(path)-5:] != ".tmpl" {
			return nil
		}
		body, err := fs.ReadFile(fsys, path)
		if err != nil {
			return fmt.Errorf("read %s: %w", path, err)
		}
		// template name is the basename without extension (e.g. "cloze")
		name := path[:len(path)-5]
		if _, err := t.New(name).Parse(string(body)); err != nil {
			return fmt.Errorf("parse %s: %w", path, err)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &Prompts{t: t}, nil
}

// Render renders the prompt template for `kind`. Returns ErrUnknownKind if
// the template is missing.
func (p *Prompts) Render(kind string, vars PromptVars) (string, error) {
	tmpl := p.t.Lookup(kind)
	if tmpl == nil {
		return "", fmt.Errorf("no prompt template for kind %q", kind)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		return "", fmt.Errorf("render %s: %w", kind, err)
	}
	return buf.String(), nil
}

// SystemPrompt is the shared system prompt used for all pregen calls. Cached
// by Anthropic so first call pays full price, subsequent calls in the same
// 5-minute window read from cache.
const SystemPrompt = `Ти си искусан учитељ српског језика који креира оригиналне граматичке вежбе, преводе и говорне вежбе за студента који:
- говори руски као матерњи језик;
- већ зна основе српског (B2);
- циља полагање испита C1;
- увежбава искључиво ћирилицу.

Правила за сваки задатак који генеришеш:
1. Сви српски текстови морају бити написани ћирилицом. Никакве латиничне варијанте.
2. Реченице треба да буду природне, идиоматске и природно звуче изворном говорнику.
3. Лексика и конструкције треба да одговарају нивоу B2–C1.
4. Сваки задатак треба да буде оригиналан — не понављај исте конструкције више пута у истој серији.
5. У објашњењу (rationale), кратко објасни граматичку поенту на ћирилици.
6. Узми у обзир разлике између српског и руског језика када их има (нпр. вид глагола, употреба инфинитива vs. да+презент).

Користи ИСКЉУЧИВО алат submit_tasks да пошаљеш генерисане задатке. Никад не одговарај текстом ван алата.`
