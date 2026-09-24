# Разработка

## Требования

- Go 1.25 или новее (минимальная версия указана в `go.mod`)
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
gofmt -l .        # список файлов, требующих форматирования
go fmt ./...      # или сразу исправить

# Статический анализ
go vet ./...
staticcheck ./... # расширенный анализ (нужно установить отдельно)

# Тесты
go test ./...

# Тесты с race detector
go test -race ./...
```

### Офлайн-запуск и песочницы

Если запись в системные кэши Go недоступна (контейнер, песочница, CI без кэша),
используйте обёртку — она держит `GOCACHE`, `GOMODCACHE` и `GOPATH` внутри
репозитория (в `.tmp/`, который добавлен в `.gitignore`):

```bash
./scripts/go.sh test ./...
./scripts/go.sh build -o makoterm .
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

- `models.go` — GORM-модели `Group` и `Host` (без `DeletedAt` — см. ниже)
- `db.go` — `InitDB()`, миграции, CRUD-функции, рекурсивное удаление
- `settings.go` — служебная таблица key/value (маркеры миграций)
- `db_test.go` — 24 теста с изолированными SQLite-базами

### sshclient/

- `config.go` — `Config`: ввод/вывод, домашний каталог, known_hosts, агент, dialer,
  логгер; значения по умолчанию для обычного запуска
- `manager.go` — `ConnectWithConfig()`, аутентификация, known_hosts, PTY, SIGWINCH,
  keepalive
- `manager_test.go` — 11 тестов с SSH-сервером, поднятым в том же процессе
- `testdata/` — тестовый ключ с парольной фразой

### ui/

- `theme.go` — все цвета и стили (палитра Kanagawa)
- `model.go` — Bubble Tea Model, Update(), View(), header/footer/help/error
- `miller.go` — трёхколоночная навигация, геометрия колонок, скроллинг
- `form.go` — формы добавления/редактирования, валидация
- `ui_test.go` — 12 тестов логики
- `layout_test.go` — 9 тестов геометрии и инвариантов рендера

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

Переход на явную передачу `*gorm.DB` — желательное направление рефакторинга: это
упростит параллельные тесты и выполнение запросов через `tea.Cmd`.

### Удаление данных

Модели **намеренно** не используют `gorm.Model`/`DeletedAt`. С ним удаление
становится «мягким» и пароль удалённого хоста остаётся в файле БД. Все удаления —
`Unscoped()` + `VACUUM` (см. `DeleteHost`, `DeleteGroup`).

### Изменение схемы

`InitDB` вызывает `AutoMigrate`, но перед этим выключает foreign keys: SQLite
пересобирает таблицу для добавления constraint и не может скопировать строки при
включённой проверке. Если добавляете таблицу или поле:

1. добавьте модель и включите её в `AutoMigrate`;
2. проверьте, что `TestInitDB_MigrateIsIdempotent` проходит — повторный запуск не
   должен пересобирать таблицы;
3. если поле меняет смысл существующих данных, напишите отдельную функцию-миграцию
   в `database/db.go` (пример — `purgeLegacySoftDeletedRows`).

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

## CI

`.github/workflows/ci.yml` запускается на каждый push и pull request и выполняет:

| Шаг | Команда |
|-----|---------|
| Форматирование | `gofmt -l .` (падает при непустом списке) |
| Аккуратность модулей | `go mod tidy` + `git diff --exit-code` |
| Анализ | `go vet ./...` |
| Тесты | `go test -race ./...` |
| Сборка | `go build -o makoterm .` (проверяет CGO) |
| Расширенный анализ | `staticcheck ./...` |

Матрица версий Go: `1.25.x` (минимум) и `1.27.x` (текущая).

## Инструменты

В проекте отсутствуют:

- Makefile
- Dockerfile
- Конфигурация линтера

Для проверки кода используйте стандартные инструменты Go и, при желании,
`staticcheck` — как в CI. Любая новая проверка должна сначала пройти локально
(`./scripts/go.sh vet ./...`, `./scripts/go.sh test -race ./...`).
