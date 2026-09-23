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

# Расширенный анализ (необязательно)
staticcheck ./...
```

В песочнице или офлайн-окружении, где нельзя писать в системные кэши Go, используйте обёртку:

```bash
./scripts/go.sh test ./...
```

## Покрытие

Всего 56 тестовых функций:

| Пакет | Файл | Тестов | Что покрыто |
|-------|------|--------|-------------|
| `database` | `database/db_test.go` | 24 | Миграции, CRUD, защита корня, физическое удаление |
| `sshclient` | `sshclient/manager_test.go` | 11 | Аутентификация, known_hosts, TOFU, зашифрованные ключи |
| `ui` | `ui/ui_test.go` | 12 | Валидация форм, навигация, состояния модели |
| `ui` | `ui/layout_test.go` | 9 | Геометрия колонок, инварианты рендера, скроллинг |

## Тесты базы данных

Файл: `database/db_test.go`

Каждый тест создаёт изолированную SQLite-базу в `t.TempDir()` через `setupTestDB(t)`.

### Инициализация и схема

| Тест | Описание |
|------|----------|
| `TestInitDB_CreatesFile` | Файл БД создаётся, права 0600 |
| `TestInitDB_SecuresWALSidecars` | WAL-файлы тоже 0600 (если созданы) |
| `TestInitDB_SeedsData` | Демо-данные: 2 группы + 3 хоста, корень скрыт |
| `TestInitDB_SeedsOnlyOnce` | После удаления групп демо-данные не возвращаются |
| `TestInitDB_NoDemoEnv` | `MAKOTERM_NO_DEMO=1` отключает демо-данные, корень остаётся |
| `TestInitDB_MigrateIsIdempotent` | Повторный запуск не пересобирает таблицы |
| `TestInitDB_EnforcesForeignKeys` | Foreign keys включены после миграции |
| `TestInitDB_MigratesLegacySoftDeletes` | База старого формата: строки вычищаются, живые данные целы |
| `TestInitDB_RepairsOrphanAndMissingRoot` | Восстановление БД без корня и с осиротевшими группами |

### Группы

| Тест | Описание |
|------|----------|
| `TestGetRootGroup` | Корневая группа с `ParentID = nil` |
| `TestGetRootGroups_HidesRoot` | Корень не попадает в список UI |
| `TestCreateGroup_AssignsRootParent` | Новые группы получают корень как родителя |
| `TestUpdateGroup` | Переименование группы |
| `TestUpdateGroup_RejectsRoot` | Корень нельзя переименовать |
| `TestDeleteGroup_RejectsRoot` | Корень нельзя удалить, данные не теряются |
| `TestDeleteGroup_Recursive` | Каскадное удаление хостов группы |
| `TestDeleteGroup_RemovesNestedGroups` | Рекурсия по вложенным группам |
| `TestGetGroupChildren` | Дочерние группы по parentID |
| `TestGetHostsForGroup_Empty` | Пустая группа возвращает 0 хостов |

### Хосты и удаление

| Тест | Описание |
|------|----------|
| `TestCreateHost` | Создание хоста в группе |
| `TestUpdateHost` | Обновление полей хоста |
| `TestDeleteHost` | Удаление одного хоста |
| `TestDeleteHost_LeavesNoPasswordBehind` | Удалённый пароль не остаётся даже в `Unscoped` выборке |
| `TestDeleteGroup_LeavesNoPasswordBehind` | То же для хостов удалённой группы |

## Тесты SSH-клиента

Файл: `sshclient/manager_test.go`

Тесты поднимают **настоящий SSH-сервер в том же процессе** (`ssh.NewServerConn` на
loopback-порту) и подключаются к нему через `ConnectWithConfig`. Это покрывает код,
который раньше не был покрыт вообще.

| Тест | Описание |
|------|----------|
| `TestConnect_TOFUAddsHostKey` | Неизвестный ключ → запрос → запись в known_hosts |
| `TestConnect_KnownHostSkipsPrompt` | Известный ключ → подключение без вопросов |
| `TestConnect_RefusesChangedHostKey` | Другой ключ → отказ, печатаются оба отпечатка |
| `TestConnect_RejectsHostKeyOnNo` | Ответ `no` → отказ, файл не изменён |
| `TestConnect_AuthenticationFailure` | Ошибка аутентификации оборачивается с адресом |
| `TestConnect_SendsNonexistentAddress` | Ошибка dial содержит адрес |
| `TestNormalizeKnownHostsEntry` | Порт 22 — без скобок, иначе `[host]:port` |
| `TestExpandHome` | Раскрытие `~` в пути к ключу |
| `TestLoadKeyInteractive_EncryptedKeyUsedToBeSkipped` | Зашифрованный ключ: явная фраза, интерактивный ввод, неверная фраза, отключённый запрос |
| `TestLoadKeyInteractive_UnparseableKey` | Некорректный файл ключа даёт ошибку |
| `TestLoadKeyInteractive_MissingFile` | Отсутствующий файл даёт ошибку |

Фикстура `sshclient/testdata/id_ed25519_passphrase` — ключ, защищённый парольной
фразой `makoterm-test-passphrase`. Это тестовый ключ, он не используется нигде
кроме тестов.

## Тесты UI

Файл: `ui/ui_test.go` — тесты в пакете `ui` (не `ui_test`), что даёт доступ к
неэкспортируемым полям (`inputs`, `state`, `pending`).

### Формы

| Тест | Описание |
|------|----------|
| `TestFormSave_GroupAdd_EmptyName` | Пустое имя → ошибка |
| `TestFormSave_GroupAdd_Valid` | Корректное создание группы |
| `TestFormSave_HostAdd_EmptyAddress` | Пустой адрес → ошибка |
| `TestFormSave_HostAdd_InvalidPort` | 5 подтестов: пустой / 0 / отрицательный / >65535 / нечисловой |
| `TestFormSave_HostAdd_Valid` | Корректное создание хоста со всеми полями |
| `TestFormSave_HostEdit_Valid` | Редактирование обновляет все поля |

### MillerColumns и модель

| Тест | Описание |
|------|----------|
| `TestMillerColumns_Navigation` | MoveUp/Down/Right/Left, границы курсора |
| `TestMillerColumns_EmptyState` | Пустые данные: SelectedGroup/Host → nil, нет паники |
| `TestMillerColumns_ClampCursors` | Курсор зажимается при уменьшении данных |
| `TestMillerColumns_View_NoZeroSize` | Width=0 или Height=0 → пустая строка |
| `TestModel_QuitState` | Ctrl+C → ShouldQuit=true, tea.Quit |
| `TestModel_DeleteConfirmation` | `d` → StateConfirmDelete, `n` → StateNormal |

## Тесты layout (инварианты рендера)

Файл: `ui/layout_test.go`

Эти тесты кодируют правило «UI никогда не выше терминала» — именно его нарушение
было причиной нескольких багов отображения, потому что `lipgloss.Height()` задаёт
минимум, а не максимум.

| Тест | Описание |
|------|----------|
| `TestView_NeverExceedsTerminal` | 7 размеров терминала, включая 10×3 |
| `TestView_ScrollIndicatorsDoNotOverflow` | Прокрученный список с `↑` и `↓` не выходит за экран |
| `TestMillerColumns_WidthIsRespected` | Длинные имена не расширяют колонку |
| `TestMillerColumns_OffsetFollowsCursorUp` | Курсор не уходит выше окна при движении вверх |
| `TestMillerColumns_ClampOffsetsRecoversAfterResize` | После увеличения окна курсор снова видим |
| `TestMillerColumns_NoBlankRowsAtTail` | Последняя страница заполнена, без пустых строк |
| `TestRenderFooter_FitsNarrowTerminal` | Footer помещается в ширину 20–120, всегда 1 строка |
| `TestForm_UpdateWithNoFields` | Форма без полей не паникует |
| `TestModel_AddWithoutTargetStaysNormal` | `a`/`e` без цели не открывают форму |

## Изоляция тестов

Тесты базы данных используют `setupTestDB(t)`:

```go
func setupTestDB(t *testing.T) func() {
    t.Helper()
    dir := t.TempDir()
    path := filepath.Join(dir, "test.db")
    err := InitDB(path)
    if err != nil {
        t.Fatalf("InitDB: %v", err)
    }
    return func() {
        if DB != nil {
            if sqlDB, err := DB.DB(); err == nil {
                sqlDB.Close()
            }
        }
        DB = nil
    }
}
```

Каждый тест получает свою изолированную копию SQLite. Глобальная переменная `database.DB` перезаписывается.

Тесты SSH не обращаются к реальному окружению: `Config` задаёт `HomeDir` во
временном каталоге, `NoAgent: true`, `SkipRawMode: true` и свой dialer, поэтому ни
ключи пользователя, ни его `known_hosts`, ни реальная сеть не затрагиваются.

## Не покрыто тестами

| Компонент | Причина |
|-----------|---------|
| Полная интеграция с реальным SSH-сервером (PTY, интерактивная сессия) | Требует настоящий `sshd` и терминал; PTY-часть проверяется вручную |
| Интерактивные приглашения в TTY-режиме | `term.ReadPassword` требует терминал |
| Рендеринг в реальном терминале | Проверяется инвариантами высоты/ширины и вручную |

## CI

`.github/workflows/ci.yml` выполняет на каждый push и pull request:

1. проверку `gofmt`;
2. отсутствие расхождений после `go mod tidy`;
3. `go vet ./...`;
4. `go test -race ./...`;
5. сборку (`go build`), что проверяет и CGO-часть SQLite;
6. `staticcheck ./...` в отдельной задаче.

Матрица версий Go: минимальная поддерживаемая (`1.24.x`) и текущая (`1.27.x`).

## Добавление тестов

### Для базы данных

Добавляйте тесты в `database/db_test.go`. Используйте `setupTestDB(t)` в начале теста.

### Для UI

Добавляйте тесты в `ui/ui_test.go` (логика) или `ui/layout_test.go` (геометрия).
Для тестирования `Update()` создавайте `tea.KeyMsg` и проверяйте состояние модели:

```go
func TestModel_NewFeature(t *testing.T) {
    m := sizedModel(t, 120, 30) // база + размер терминала

    msg := tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}
    result, _ := m.Update(msg)

    if result.(Model).state != StateExpected {
        t.Errorf("expected StateExpected, got %d", result.(Model).state)
    }
}
```

Если тест касается рендеринга, добавьте проверку инварианта — например, что
`lipgloss.Height(m.View())` не превышает высоту терминала.

### Для SSH

Расширяйте `sshclient/manager_test.go`: `startTestServer` уже умеет принимать
соединения, сессии, PTY и завершать shell корректным `exit-status`. Для новых
сценариев достаточно добавить опции в `serverOptions` или подготовить файл
`known_hosts` во временном каталоге.
