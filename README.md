# MakoTerm3 🦈

A lightning-fast, aesthetic, and fully-featured Terminal UI (TUI) SSH client built with Go and Bubble Tea. MakoTerm3 combines an integrated SQLite database, a Miller Columns tree navigation for your hosts, and a seamless native SSH multiplexer into a single, highly portable binary.

## Features ✨

- **Native SSH Client**: Powered by `golang.org/x/crypto/ssh`. No need to rely on external `ssh` binaries.
- **Smart Authentication**: Automatically supports `ssh-agent`, local keys (`~/.ssh/id_rsa`, `~/.ssh/id_ed25519`), with database password fallback.
- **Miller Columns Navigation**: Browse your server groups and hosts with an elegant, Ranger-like layout.
- **Pagination & Scrolling**: Fluidly handles hundreds of servers with automatic list pagination and offset tracking.
- **Integrated Database**: Safely store your server groups, addresses, users, and passwords using SQLite + GORM.
- **TUI Multiplexer**: Instantly jump in and out of SSH sessions without losing your UI state. Perfect for backgrounding sessions.
- **Kanagawa Theme & Nerd Fonts**: Soft, aesthetic, and eye-pleasing dark theme integrated via `lipgloss` with beautiful 📁 and 🖥️ icons.
- **Built-in CRUD**: Add, edit, and delete hosts and groups directly from the TUI with beautiful inline forms.

## Installation 🚀

You can build MakoTerm3 from source. Make sure you have Go (1.20+) installed.

```bash
# Clone the repository
git clone https://github.com/yourusername/makoterm3.git
cd makoterm3

# Download dependencies and build
go mod tidy
go build -o makoterm

# Run the client
./makoterm
```

## Usage & Keybindings ⌨️

By default, the SQLite database is created in your home directory: `~/.makoterm.db`. The application automatically seeds a `Root` group to get you started.

### Navigation
- `↑` / `k` : Move cursor up
- `↓` / `j` : Move cursor down
- `←` / `h` : Move to previous column (Groups)
- `→` / `l` : Move to next column (Hosts)
- `Enter`   : Connect to the selected host (if in the Hosts column)

### Management (CRUD)
- `a` : **Add**. Adds a new group (if in Groups column) or a new host (if in Hosts column).
- `e` : **Edit**. Edits the currently selected group or host.
- `d` : **Delete**. Deletes the currently selected group or host.

### Form Navigation
- `Tab` / `↓` : Next field
- `Shift+Tab` / `↑` : Previous field
- `Enter` : Save and submit form
- `Esc` : Cancel and return to navigation

### Quitting
- `q` / `Ctrl+C` : Quit the application

## Screenshots 📸

*(Add screenshots of your UI here! Show the Kanagawa theme, the forms, and the SSH session)*

## License 📜

Distributed under the MIT License. See `LICENSE` for more information.
