package server

import (
	"net/http"
	"strconv"
)

// /admin/review serves a self-contained admin page for reviewing pregen tasks.
// The JS uses /api/admin/tasks (list, flag, delete).
func (s *Server) handleAdminReview(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(adminReviewHTML))
}

type adminTaskRow struct {
	ID         int64  `json:"id"`
	Kind       string `json:"kind"`
	Difficulty int    `json:"difficulty"`
	Topic      string `json:"topic"`
	Prompt     string `json:"prompt"`
	Payload    string `json:"payload"`
	Expected   string `json:"expected"`
	Rationale  string `json:"rationale"`
	Source     string `json:"source"`
	Flagged    bool   `json:"flagged"`
	CreatedAt  int64  `json:"created_at"`
}

func (s *Server) handleAdminListTasks(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	source := r.URL.Query().Get("source")
	if source == "" {
		source = "pregen:claude%"
	}
	limit := 200
	if l := r.URL.Query().Get("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= 1000 {
			limit = n
		}
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, kind, difficulty, COALESCE(topic,''), prompt, payload_json, expected_json,
		       COALESCE(rationale,''), source, flagged, created_at
		FROM tasks
		WHERE source LIKE ?
		ORDER BY id DESC
		LIMIT ?
	`, source, limit)
	if err != nil {
		httpError(w, "query tasks", err)
		return
	}
	defer rows.Close()
	var out []adminTaskRow
	for rows.Next() {
		var t adminTaskRow
		var flagged int
		if err := rows.Scan(&t.ID, &t.Kind, &t.Difficulty, &t.Topic, &t.Prompt,
			&t.Payload, &t.Expected, &t.Rationale, &t.Source, &flagged, &t.CreatedAt); err != nil {
			httpError(w, "scan", err)
			return
		}
		t.Flagged = flagged != 0
		out = append(out, t)
	}
	writeJSON(w, http.StatusOK, map[string]any{"tasks": out})
}

func (s *Server) handleAdminFlagTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if _, err := s.db.ExecContext(ctx,
		`UPDATE tasks SET flagged = CASE flagged WHEN 0 THEN 1 ELSE 0 END WHERE id = ?`, id); err != nil {
		httpError(w, "flag", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleAdminDeleteTask(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if _, err := s.db.ExecContext(ctx, `DELETE FROM tasks WHERE id = ?`, id); err != nil {
		httpError(w, "delete", err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

const adminReviewHTML = `<!doctype html>
<html lang="sr-Cyrl-RS">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
<title>Преглед задатака</title>
<style>
  body { font: 15px/1.5 -apple-system, system-ui, sans-serif; margin: 0; padding: 1rem; background: #f7f5f1; color: #1a1a1a; }
  h1 { color: #0a3d62; font-size: 1.3rem; margin: 0 0 1rem; }
  .row { background: #fff; border: 1px solid #e3e0d8; border-radius: 12px; padding: 0.8rem 1rem; margin-bottom: 0.7rem; }
  .row.flagged { background: #fff8e1; border-color: #f0c14b; }
  .meta { color: #6b6b6b; font-size: 0.85rem; display: flex; flex-wrap: wrap; gap: 0.6rem; margin-bottom: 0.4rem; }
  .pill { background: #e0e7ef; color: #0a3d62; padding: 0.1em 0.6em; border-radius: 999px; font-size: 0.8rem; }
  .prompt { font-size: 1.05rem; font-weight: 500; margin: 0.2rem 0; }
  pre { background: #f0ede4; border-radius: 6px; padding: 0.5em 0.8em; overflow-x: auto; font-size: 0.85rem; margin: 0.4rem 0; white-space: pre-wrap; word-break: break-word; }
  .actions { display: flex; gap: 0.5rem; margin-top: 0.5rem; }
  button { font: inherit; border: 1px solid #ccc; background: #fff; border-radius: 8px; padding: 0.4em 0.9em; cursor: pointer; }
  button.danger { color: #c62828; border-color: #c62828; }
  button.primary { background: #0a3d62; color: #fff; border: 0; }
  .empty { color: #6b6b6b; font-style: italic; padding: 2em; text-align: center; }
  .filter { margin-bottom: 1rem; display: flex; gap: 0.5rem; align-items: center; }
  input[type="text"] { padding: 0.4em 0.8em; border: 1px solid #ccc; border-radius: 8px; }
</style>
</head>
<body>
<h1>Преглед задатака</h1>
<div class="filter">
  <label>Извор:</label>
  <input id="source" type="text" value="pregen:claude%">
  <button class="primary" onclick="load()">Освежи</button>
</div>
<div id="list"></div>

<script>
async function load() {
  const source = document.getElementById('source').value;
  const r = await fetch('/api/admin/tasks?source=' + encodeURIComponent(source));
  const data = await r.json();
  const list = document.getElementById('list');
  list.innerHTML = '';
  if (!data.tasks || !data.tasks.length) {
    list.innerHTML = '<div class="empty">Нема задатака за изабрани извор.</div>';
    return;
  }
  for (const t of data.tasks) {
    const row = document.createElement('div');
    row.className = 'row' + (t.flagged ? ' flagged' : '');
    row.innerHTML = '<div class="meta">' +
      '<span class="pill">#' + t.id + '</span>' +
      '<span class="pill">' + t.kind + '</span>' +
      '<span class="pill">D' + t.difficulty + '</span>' +
      (t.topic ? '<span class="pill">' + escapeHTML(t.topic) + '</span>' : '') +
      (t.flagged ? '<span class="pill">⚠ означено</span>' : '') +
      '</div>' +
      '<div class="prompt">' + escapeHTML(t.prompt) + '</div>' +
      '<pre>payload: ' + escapeHTML(t.payload) + '</pre>' +
      '<pre>expected: ' + escapeHTML(t.expected) + '</pre>' +
      (t.rationale ? '<pre>rationale: ' + escapeHTML(t.rationale) + '</pre>' : '') +
      '<div class="actions">' +
      '<button onclick="toggleFlag(' + t.id + ')">' + (t.flagged ? 'Уклони ознаку' : 'Означи') + '</button>' +
      '<button class="danger" onclick="del(' + t.id + ')">Обриши</button>' +
      '</div>';
    list.appendChild(row);
  }
}
async function toggleFlag(id) {
  await fetch('/api/admin/tasks/' + id + '/flag', { method: 'POST' });
  await load();
}
async function del(id) {
  if (!confirm('Обрисати задатак #' + id + '?')) return;
  await fetch('/api/admin/tasks/' + id, { method: 'DELETE' });
  await load();
}
function escapeHTML(s) {
  return String(s).replace(/[&<>"']/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;',"'":'&#39;'}[c]));
}
load();
</script>
</body>
</html>`
