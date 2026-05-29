package ui

import (
	"fmt"
	"strconv"
	"strings"

	"makoterm/database"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
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
	focus  int
}

func NewForm(fType FormType, group *database.Group, host *database.Host) Form {
	var inputs []textinput.Model

	switch fType {
	case FormTypeGroupAdd, FormTypeGroupEdit:
		name := textinput.New()
		name.Placeholder = "Group Name"
		name.Focus()
		if fType == FormTypeGroupEdit && group != nil {
			name.SetValue(group.Name)
		}
		inputs = append(inputs, name)

	case FormTypeHostAdd, FormTypeHostEdit:
		name := textinput.New()
		name.Placeholder = "Host Name (e.g. Prod DB)"
		name.Focus()

		addr := textinput.New()
		addr.Placeholder = "Address (e.g. 192.168.1.100)"

		port := textinput.New()
		port.Placeholder = "Port (e.g. 22)"
		port.SetValue("22")

		user := textinput.New()
		user.Placeholder = "User (e.g. root)"

		pass := textinput.New()
		pass.Placeholder = "Password (optional)"
		pass.EchoMode = textinput.EchoPassword
		pass.EchoCharacter = '•'

		if fType == FormTypeHostEdit && host != nil {
			name.SetValue(host.Name)
			addr.SetValue(host.Address)
			port.SetValue(strconv.Itoa(host.Port))
			user.SetValue(host.User)
			pass.SetValue(host.Password)
		}

		inputs = append(inputs, name, addr, port, user, pass)
	}

	return Form{
		Type:        fType,
		TargetGroup: group,
		TargetHost:  host,
		inputs:      inputs,
		focus:       0,
	}
}

func (f *Form) Update(msg tea.Msg) (Form, tea.Cmd) {
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

	b.WriteString(TitleStyle.Render(title))
	b.WriteString("\n\n")

	for i := range f.inputs {
		b.WriteString(f.inputs[i].View())
		b.WriteString("\n\n")
	}

	b.WriteString(HelpStyle.Render("tab/shift+tab: next/prev field • enter: save • esc: cancel"))

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(CrystalBlue).
		Padding(1, 2).
		Render(b.String())
}

// Save executes the database save operation based on the form inputs
func (f *Form) Save() error {
	switch f.Type {
	case FormTypeGroupAdd:
		g := &database.Group{Name: f.inputs[0].Value()}
		// If we are creating a subgroup, we'd set ParentID. For now, we only have root groups in UI.
		// Wait, we seeded a root group with ID=1. Let's make it a child of ID=1 if possible.
		// Or just leave parent_id null so it appears in GetRootGroups.
		return database.CreateGroup(g)

	case FormTypeGroupEdit:
		if f.TargetGroup != nil {
			f.TargetGroup.Name = f.inputs[0].Value()
			return database.UpdateGroup(f.TargetGroup)
		}

	case FormTypeHostAdd:
		port, _ := strconv.Atoi(f.inputs[2].Value())
		h := &database.Host{
			GroupID:  &f.TargetGroup.ID,
			Name:     f.inputs[0].Value(),
			Address:  f.inputs[1].Value(),
			Port:     port,
			User:     f.inputs[3].Value(),
			Password: f.inputs[4].Value(),
		}
		return database.CreateHost(h)

	case FormTypeHostEdit:
		if f.TargetHost != nil {
			port, _ := strconv.Atoi(f.inputs[2].Value())
			f.TargetHost.Name = f.inputs[0].Value()
			f.TargetHost.Address = f.inputs[1].Value()
			f.TargetHost.Port = port
			f.TargetHost.User = f.inputs[3].Value()
			f.TargetHost.Password = f.inputs[4].Value()
			return database.UpdateHost(f.TargetHost)
		}
	}
	return fmt.Errorf("unknown form type or missing target")
}
