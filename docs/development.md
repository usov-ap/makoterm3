# Разработка

## Требования

- Go 1.26.3 или новее (указано в `go.mod`)
- C компилятор (GCC или Clang) — для CGO-зависимости SQLite
- Git

## Получение исходников

```bash
git clone https://github.com/usov-ap/makoterm3.git
cd makoterm3
```

## Установка зависимостей

```bash
go mod download
```

Или:

```bash
go mod tidy
```

## Сборка

```bash
go build -o makoterm
```

## Запуск

```bash
./makoterm
```

Или напрямую из исходников:

```bash
go run .
```

## Проверка кода

```bash
# Форматирование
go fmt ./...

# Статический анализ
go vet ./...

# Тесты
go test ./...

# Тесты с race detector
go test -race ./...
```

## Структура пакетов

| Пакет | Путь | Назначение | Зависимости |
|-------|------|------------|-------------|
| `main` | `main.go` | Точка входа, цикл запуска | database, sshclient, ui |
| `database` | `database/` | SQLite через GORM, CRUD | gorm, sqlite |
| `sshclient` | `sshclient/` | SSH-подключение | x/crypto/ssh, database (Host struct) |
| `ui` | `ui/` | Bubble Tea TUI | bubbletea, lipgloss, bubbles, database |

## Организация кода

### main.go

Точка входа. Содержит цикл exit-and-restart:

1. `database.InitDB("")` — инициализация SQLite
2. `ui.InitialModel()` — загрузка данных
3. Цикл: создать `tea.Program` → `Run()` → проверить `ShouldQuit` / `SelectedToConnect` → SSH или выход

### database/

- `models.go` — GORM-модели `Group` и `Host`
- `db.go` — `InitDB()`, CRUD-функции, рекурсивное удаление
- `db_test.go` — 10 тестов с изолированными SQLite-базами

### sshclient/

- `manager.go` — функция `Connect(host)`, аутентификация, known_hosts, PTY, SIGWINCH

### ui/

- `theme.go` — все цвета и стили (палитра Kanagawa)
- `model.go` — Bubble Tea Model, Update(), View(), header/footer/help/error
- `miller.go` — трёхколоночная навигация, скроллинг
- `form.go` — формы добавления/редактирования, валидация
- `ui_test.go` — 12 тестов

## Соглашения по коду

### Импорты

Три группы, разделённые пустой строкой:

```go
import (
    "fmt"           // стандартная библиотека
    "strings"

    "makoterm/database"  // проект

    "github.com/charmbracelet/bubbletea"  // внешние
)
```

### Ошибки

Используется `fmt.Errorf("context: %w", err)` для оборачивания ошибок.

### Стили

Все стили определяются в `ui/theme.go`. Не создавайте `lipgloss.NewStyle()` в других файлах.

### Глобальное состояние

`database.DB` — глобальная переменная. В тестах подменяется через `setupTestDB(t)`.

## Добавление нового экрана

1. Добавьте новое значение в `State` (в `model.go`)
2. Обработайте вход в новое состояние в `Update()` (клавиша → `m.state = StateNew`)
3. Добавьте обработку клавиш в новом состоянии (в начале `Update()`)
4. Добавьте рендеринг в `View()` (case в switch по state)
5. Обновите footer в `renderFooter()` для нового состояния
6. Добавьте стили в `theme.go` если нужны новые визуальные элементы

## Добавление нового поля хоста

1. Добавьте поле в `database.Host` (`models.go`)
2. Добавьте `textinput.New()` и label в `NewForm()` (`form.go`)
3. Обработайте новое поле в `Save()` (`form.go`)
4. Отобразите в `renderDetails()` (`miller.go`)
5. Добавьте тест в `ui_test.go`

## Инструменты

В проекте отсутствуют:

- Makefile
- Dockerfile
- CI/CD конфигурация
- Linter-конфигурация (staticcheck, golangci-lint)

Для проверки кода используйте стандартные инструменты Go (`go fmt`, `go vet`, `go test`).
