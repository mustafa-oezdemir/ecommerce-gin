### Projektübersicht
**Ecommerce Gin** ist eine Entwicklungs‑Version eines kleinen E‑Commerce Backends mit **Go**, **Gin** und **GORM**. Der Code enthält HTTP‑Handler, HTML‑Templates, Docker Compose für MySQL und ein CLI‑Seed‑Tool zum Anlegen von Testbenutzern. Der aktuelle Stand: Datenbank und Seed‑Tool sind vorbereitet, Templates liegen im Projekt, Docker Compose benötigt optional eine `app`‑Service‑Ergänzung für konsistente Docker‑Ausführung.

---

### Schnellstart
**Lokale Entwicklung schnell**
```powershell
# Projektverzeichnis
cd D:\Code\Go\ecommerce-gin

# Docker MySQL starten
docker-compose up -d

# Lokale Umgebungsvariable setzen und Server starten
$env:MYSQL_DSN="pehlione:delidolu57@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"
go run cmd/server/main.go
```

**Docker Compose komplett**
```bash
docker-compose down -v
docker-compose up --build
```

---

### Umgebungsvariablen
**Empfohlene `.env` Datei**
```env
MYSQL_ROOT_PASSWORD=root
MYSQL_DATABASE=ecommerce
MYSQL_USER=pehlione
MYSQL_PASSWORD=delidolu57

# Lokal beim Entwickeln
MYSQL_HOST=127.0.0.1
MYSQL_PORT=3307

GIN_MODE=release
PORT=8080
```
**Hinweis**: Für lokale Ausführung mit Docker ist `3307:3306` empfohlen, damit Host‑MySQL‑Konflikte vermieden werden. Die Anwendung sollte die DSN aus den Einzelvariablen zusammensetzen oder `MYSQL_DSN` direkt setzen.

---

### Datenbank Seeding CLI
**Datei** `cmd/seed/main.go` erstellt drei Benutzer mit Rollen **admin**, **employee**, **customer** und hasht Passwörter mit bcrypt.  
**Ausführen lokal**
```powershell
$env:MYSQL_DSN="pehlione:delidolu57@tcp(127.0.0.1:3307)/ecommerce?charset=utf8mb4&parseTime=True&loc=Local"
go run cmd/seed/main.go
```
**Docker Compose Option**: Füge einen `app` Service hinzu und starte das Seed‑Skript im Container:
```bash
docker-compose run --rm app sh -c "go run cmd/seed/main.go"
```

---

### Templates und statische Dateien
**Pfadabgleich**: Der Server lädt Templates per `LoadHTMLGlob`. Stelle sicher, dass die Template‑Dateien am erwarteten Ort liegen oder passe den Pfad an.  
**Empfohlene Struktur**
```
web/templates/
  layout.tmpl
  product_list.tmpl
  login.tmpl
  cart.tmpl
```
**Sofortlösung**: Entweder die Dateien nach `internal/web/templates` verschieben oder `main.go` so ändern:
```go
r.LoadHTMLGlob("web/templates/*.tmpl")
```
**Robuste Lösung**: Go `embed` verwenden, um Templates in das Binary zu packen und Pfadabhängigkeiten zu eliminieren.

---

### Roadmap und nächste Schritte
**Kurzfristig**
- Templates mit `embed` einbinden und `LoadHTMLGlob` entfernen.  
- `docker-compose.yml` um `app` Service erweitern für reproduzierbare Docker‑Umgebung.  
**Mittelfristig**
- RBAC Middleware implementieren (Route‑Schutz für admin/employee/customer).  
- Integrationstests für Migrationen und Seed‑Skript.  
**Langfristig**
- Secrets Management für Produktionsumgebungen, TLS, Health Checks und Logging verbessern.

