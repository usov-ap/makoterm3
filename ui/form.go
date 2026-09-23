package ui

import (
	"fmt"
	"strconv"
	"strings"

	"makoterm/database"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type FormType int

const (
	FormTypeGroupAdd FormType = iota
	FormTypeGroupEdit
	FormTypeHostAdd
	FormTypeHostEdit
)

type Form struct {
	Type        FormType
	TargetGroup *database.Group
	TargetHost  *database.Host

	inputs []textinput.Model
	labels []string
	focus  int
}

func NewForm(fType FormType, group *database.Group, host *database.Host) Form {
	var inputs []textinput.Model
	var labels []string

	switch fType {
	case FormTypeGroupAdd, FormTypeGroupEdit:
		name := textinput.New()
		name.Placeholder = "Group Name"
		name.Focus()
		if fType == FormTypeGroupEdit && group != nil {
			name.SetValue(group.Name)
		}
		inputs = append(inputs, name)
		labels = append(labels, "Name")

	case FormTypeHostAdd, FormTypeHostEdit:
		name := textinput.New()
		name.Placeholder = "e.g. Production DB"
		name.Focus()

		addr := textinput.New()
		addr.Placeholder = "e.g. 192.168.1.100"

		port := textinput.New()
		port.Placeholder = "e.g. 22"
		port.SetValue("22")

		user := textinput.New()
		user.Placeholder = "e.g. root"

		pass := textinput.New()
		pass.Placeholder = "optional, prefer SSH keys"
		pass.EchoMode = textinput.EchoPassword
		pass.EchoCharacter = '•'

		keyPath := textinput.New()
		keyPath.Placeholder = "e.g. ~/.ssh/id_rsa"

		if fType == FormTypeHostEdit && host != nil {
			name.SetValue(host.Name)
			addr.SetValue(host.Address)
			port.SetValue(strconv.Itoa(host.Port))
			user.SetValue(host.User)
			pass.SetValue(host.Password)
			keyPath.SetValue(host.KeyPath)
		}

		inputs = append(inputs, name, addr, port, user, pass, keyPath)
		labels = append(labels, "Name", "Address", "Port", "User", "Password", "Key Path")
	}

	return Form{
		Type:        fType,
		TargetGroup: group,
		TargetHost:  host,
		inputs:      inputs,
		labels:      labels,
		focus:       0,
	}
}

func (f *Form) Update(msg tea.Msg) (Form, tea.Cmd) {
	// A form always has at least one field, but guard anyway: the focus
	// arithmetic below divides by len(inputs) and would panic.
	if len(f.inputs) == 0 {
		return *f, nil
	}

	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "tab", "down":
			f.focus = (f.focus + 1) % len(f.inputs)
			for i := range f.inputs {
				if i == f.focus {
					cmds = append(cmds, f.inputs[i].Focus())
				} else {
					f.inputs[i].Blur()
				}
			}
		case "shift+tab", "up":
			f.focus--
			if f.focus < 0 {
				f.focus = len(f.inputs) - 1
			}
			for i := range f.inputs {
				if i == f.focus {
					cmds = append(cmds, f.inputs[i].Focus())
				} else {
					f.inputs[i].Blur()
				}
			}
		}
	}

	for i := range f.inputs {
		var cmd tea.Cmd
		f.inputs[i], cmd = f.inputs[i].Update(msg)
		if cmd != nil {
			cmds = append(cmds, cmd)
		}
	}

	return *f, tea.Batch(cmds...)
}

func (f Form) View() string {
	var b strings.Builder

	title := "Add Group"
	switch f.Type {
	case FormTypeGroupEdit:
		title = "Edit Group"
	case FormTypeHostAdd:
		title = "Add Host"
	case FormTypeHostEdit:
		title = "Edit Host"
	}

	b.WriteString(FormTitleStyle.Render(title))
	b.WriteString("\n\n")

	for i := range f.inputs {
		b.WriteString(FormLabelStyle.Render(f.labels[i]))
		b.WriteString("\n")
		b.WriteString(f.inputs[i].View())
		b.WriteString("\n\n")
	}

	b.WriteString(MutedStyle.Render("Tab next • Shift+Tab prev • Enter save • Esc cancel"))

	return DialogStyle.Render(b.String())
}

// Save validates inputs and executes the database save operation.
func (f *Form) Save() error {
	switch f.Type {
	case FormTypeGroupAdd:
		name := strings.TrimSpace(f.inputs[0].Value())
		if name == "" {
			return fmt.Errorf("group name cannot be empty")
		}
		// Make new groups children of the root group
		root, err := database.GetRootGroup()
		if err != nil {
			g := &database.Group{Name: name}
			return database.CreateGroup(g)
		}
		g := &database.Group{Name: name, ParentID: &root.ID}
		return database.CreateGroup(g)

	case FormTypeGroupEdit:
		if f.TargetGroup != nil {
			name := strings.TrimSpace(f.inputs[0].Value())
			if name == "" {
				return fmt.Errorf("group name cannot be empty")
			}
			f.TargetGroup.Name = name
			return database.UpdateGroup(f.TargetGroup)
		}

	case FormTypeHostAdd:
		name := strings.TrimSpace(f.inputs[0].Value())
		addr := strings.TrimSpace(f.inputs[1].Value())
		if name == "" {
			return fmt.Errorf("host name cannot be empty")
		}
		if addr == "" {
			return fmt.Errorf("address cannot be empty")
		}
		port, err := strconv.Atoi(strings.TrimSpace(f.inputs[2].Value()))
		if err != nil || port < 1 || port > 65535 {
			return fmt.Errorf("port must be a number between 1 and 65535")
		}
		h := &database.Host{
			GroupID:  &f.TargetGroup.ID,
			Name:     name,
			Address:  addr,
			Port:     port,
			User:     strings.TrimSpace(f.inputs[3].Value()),
			Password: f.inputs[4].Value(),
			KeyPath:  strings.TrimSpace(f.inputs[5].Value()),
		}
		return database.CreateHost(h)

	case FormTypeHostEdit:
		if f.TargetHost != nil {
			name := strings.TrimSpace(f.inputs[0].Value())
			addr := strings.TrimSpace(f.inputs[1].Value())
			if name == "" {
				return fmt.Errorf("host name cannot be empty")
			}
			if addr == "" {
				return fmt.Errorf("address cannot be empty")
			}
			port, err := strconv.Atoi(strings.TrimSpace(f.inputs[2].Value()))
			if err != nil || port < 1 || port > 65535 {
				return fmt.Errorf("port must be a number between 1 and 65535")
			}
			f.TargetHost.Name = name
			f.TargetHost.Address = addr
			f.TargetHost.Port = port
			f.TargetHost.User = strings.TrimSpace(f.inputs[3].Value())
			f.TargetHost.Password = f.inputs[4].Value()
			f.TargetHost.KeyPath = strings.TrimSpace(f.inputs[5].Value())
			return database.UpdateHost(f.TargetHost)
		}
	}
	return fmt.Errorf("unknown form type or missing target")
}
