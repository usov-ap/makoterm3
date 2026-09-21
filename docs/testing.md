# Тестирование

## Запуск тестов

```bash
# Все тесты
go test ./...

# С подробным выводом
go test ./... -v

# С проверкой data races
go test -race ./...

# Статический анализ
go vet ./...
```

## Покрытие

Всего 22 теста: 10 в `database`, 12 в `ui`.

## Тесты базы данных

Файл: `database/db_test.go`

Каждый тест создаёт изолированную SQLite-базу в `t.TempDir()` через `setupTestDB(t)`.

| Тест | Описание |
|------|----------|
| `TestInitDB_CreatesFile` | Файл БД создаётся, права 0600 |
| `TestInitDB_SeedsData` | Демонстрационные данные: Root + 2 группы + 3 хоста |
| `TestGetRootGroup` | Корневая группа с `ParentID = nil` |
| `TestCreateGroup` | Создание группы, проверка через `GetRootGroups` |
| `TestUpdateGroup` | Переименование группы |
| `TestDeleteGroup_Recursive` | Каскадное удаление: хосты → подгруппы → группа |
| `TestCreateHost` | Создание хоста в группе |
| `TestDeleteHost` | Удаление одного хоста |
| `TestGetGroupChildren` | Дочерние группы по parentID |
| `TestGetHostsForGroup_Empty` | Пустая группа возвращает 0 хостов |

## Тесты UI

Файл: `ui/ui_test.go`

Тесты находятся в пакете `ui` (не `ui_test`), что даёт доступ к неэкспортируемым полям (`inputs`, `state`, `pending`).

### Тесты форм

| Тест | Описание |
|------|----------|
| `TestFormSave_GroupAdd_EmptyName` | Пустое имя → ошибка |
| `TestFormSave_GroupAdd_Valid` | Корректное создание группы |
| `TestFormSave_HostAdd_EmptyAddress` | Пустой адрес → ошибка |
| `TestFormSave_HostAdd_InvalidPort` | 5 подтестов: пустой / 0 / отрицательный / >65535 / нечисловой |
| `TestFormSave_HostAdd_Valid` | Корректное создание хоста со всеми полями |
| `TestFormSave_HostEdit_Valid` | Редактирование обновляет все поля |

### Тесты MillerColumns

| Тест | Описание |
|------|----------|
| `TestMillerColumns_Navigation` | MoveUp/Down/Right/Left, границы курсора |
| `TestMillerColumns_EmptyState` | Пустые данные: SelectedGroup/Host → nil, нет паники |
| `TestMillerColumns_ClampCursors` | Курсор зажимается при уменьшении данных |
| `TestMillerColumns_View_NoZeroSize` | Width=0 или Height=0 → пустая строка |

### Тесты Model

| Тест | Описание |
|------|----------|
| `TestModel_QuitState` | Ctrl+C → ShouldQuit=true, tea.Quit |
| `TestModel_DeleteConfirmation` | `d` → StateConfirmDelete, `n` → StateNormal |

## Не покрыто тестами

| Компонент | Причина |
|-----------|---------|
| `sshclient/manager.go` | Требует реального SSH-сервера |
| `View()` output | Рендеринг тестируется вручную |
| Header/Footer рендеринг | Визуальная проверка |
| Help overlay | Визуальная проверка |
| Интеграционные тесты | Отсутствуют |

## Изоляция тестов

Тесты базы данных используют `setupTestDB(t)`:

```go
func setupTestDB(t *testing.T) {
    t.Helper()
    dir := t.TempDir()
    path := filepath.Join(dir, "test.db")
    err := InitDB(path)
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
}
```

Каждый тест получает свою изолированную копию SQLite. Глобальная переменная `database.DB` перезаписывается.

UI-тесты содержат аналогичную функцию, импортирующую `database.InitDB`.

## Добавление тестов

### Для базы данных

Добавляйте тесты в `database/db_test.go`. Используйте `setupTestDB(t)` в начале теста.

### Для UI

Добавляйте тесты в `ui/ui_test.go`. Используйте `setupTestDB(t)` для инициализации БД. Для тестирования Update() создавайте `tea.KeyMsg` и проверяйте состояние модели.

Пример:

```go
func TestModel_NewFeature(t *testing.T) {
    setupTestDB(t)
    m := InitialModel()
    m.width = 120
    m.height = 30
    m.miller.Width = 120
    m.miller.Height = 28

    // Отправить клавишу
    msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
    result, _ := m.Update(msg)
    model := result.(Model)

    // Проверить состояние
    if model.state != StateExpected {
        t.Errorf("expected StateExpected, got %d", model.state)
    }
}
```
