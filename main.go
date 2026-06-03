package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/rkoesters/xdg/trash"
)

func green(s any) string  { return fmt.Sprintf("\033[32m%v\033[0m", s) }
func yellow(s any) string { return fmt.Sprintf("\033[33m%v\033[0m", s) }
func red(s any) string    { return fmt.Sprintf("\033[31m%v\033[0m", s) }
func blue(s any) string   { return fmt.Sprintf("\033[94m%v\033[0m", s) }
func cyan(s any) string   { return fmt.Sprintf("\033[36m%v\033[0m", s) }

type fileItem struct {
	name       string
	displayStr string
	isDir      bool
	size       int64
	modTime    time.Time
}
type GetFileMsg []fileItem
type sessionState int

const (
	stateNormal sessionState = iota
	stateConfirm
)

type model struct {
	files       []fileItem
	cursor      int
	cursorStart int
	deleted     map[int]struct{}
	height      int
	viewstart   int
	dangerMode  bool
	state       sessionState
	deleteQueue []int
}

func initialModel() model {
	return model{
		files:       nil,
		cursor:      0,
		cursorStart: -1,
		deleted:     make(map[int]struct{}),
		height:      -1,
		viewstart:   0,
		dangerMode:  false,
		state:       stateNormal,
		deleteQueue: nil,
	}
}

func getDirFiles() tea.Msg {
	var directoryItems []fileItem
	entries, err := os.ReadDir(".")
	if err != nil {
		log.Fatal("Not able to read directory: \n", err)
	}

	for _, entry := range entries {
		itemName := entry.Name()

		fileInfo, err := entry.Info()
		if err != nil {
			continue
		}

		fileMode := fileInfo.Mode()
		display := itemName

		if fileMode.IsDir() {
			display = blue(itemName+"/")
		} else if fileMode&os.ModeSymlink != 0 {
			display = cyan(itemName+"@")
		} else if fileMode&0111 != 0 {
			display = green(itemName+"*")
		}

		directoryItems = append(directoryItems, fileItem{
			name:       itemName,
			displayStr: display,
			isDir:      fileMode.IsDir(),
			size:       fileInfo.Size(),
			modTime:    fileInfo.ModTime(),
		})
	}

	return GetFileMsg(directoryItems)
}

func (m model) advanceDeletionQueue() (tea.Model, tea.Cmd) {
	if len(m.deleteQueue) == 0 {
		m.state = stateNormal
		m.deleted = make(map[int]struct{})
		return m, getDirFiles
	}
	return m, nil
}

func (m model) moveToTrash(itemPath string) error {
	absPath, err := filepath.Abs(itemPath)
	if err != nil {
		return err
	}

	return trash.Trash(absPath)
}


func (m model) Init() tea.Cmd {
	return getDirFiles
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.height = msg.Height
	case GetFileMsg:
		m.files = msg
	case tea.KeyPressMsg:
		if m.state == stateConfirm {
			switch msg.String() {
			case "y", "Y", "enter":
				targetIndex := m.deleteQueue[0]
				targetItem := m.files[targetIndex]
				
				m.moveToTrash(targetItem.name)
				
				m.deleteQueue = m.deleteQueue[1:]
				return m.advanceDeletionQueue()
				
			case "n", "N":
				m.deleteQueue = m.deleteQueue[1:]
				return m.advanceDeletionQueue()
				
			case "esc", "q", "ctrl+c":
				m.state = stateNormal
				m.deleteQueue = nil
				return m, nil
			}
			return m, nil
		}

		switch msg.String() {

		case "esc":
			m.cursorStart = -1
		case "ctrl+c", "q":
			return m, tea.Quit
		case "X":
			if len(m.deleted) == 0 {
				return m, nil
			}

			var activeSelections []int
			for selectedIndex := range m.deleted {
				activeSelections = append(activeSelections, selectedIndex)
			}

			if m.dangerMode {
				for _, selectedIndex := range activeSelections {
					targetItem := m.files[selectedIndex]
					os.RemoveAll(targetItem.name)
				}
				m.deleted = make(map[int]struct{})
				return m, getDirFiles
			}

			m.deleteQueue = activeSelections
			m.state = stateConfirm
			return m, nil
		case "up", "k":
			m.cursorStart = -1
			if m.cursor > 0 {
				m.cursor--
			}

			start, end := getViewBox(m.height, m.viewstart, len(m.files))

			if m.cursor < start {
				m.viewstart--
			}
			if m.cursor >= end {
				m.viewstart++
			}
		case "shift+up", "shift+k":
			if m.cursor > 0 {
				if m.cursorStart == -1 { m.cursorStart = m.cursor } 
				m.cursor--
			}

			start, end := getViewBox(m.height, m.viewstart, len(m.files))

			if m.cursor < start {
				m.viewstart--
			}
			if m.cursor >= end {
				m.viewstart++
			}
		case "down", "j":
			m.cursorStart = -1
			if m.cursor < len(m.files)-1 {
				m.cursor++
			}

			start, end := getViewBox(m.height, m.viewstart, len(m.files))

			if m.cursor < start {
				m.viewstart--
			}
			if m.cursor >= end {
				m.viewstart++
			}
		case "shift+down","shift+j":
			if m.cursor < len(m.files)-1 {
				if m.cursorStart == -1 { m.cursorStart = m.cursor } 
				m.cursor++
			}

			start, end := getViewBox(m.height, m.viewstart, len(m.files))

			if m.cursor < start {
				m.viewstart--
			}
			if m.cursor >= end {
				m.viewstart++
			}
		case "t":
			m.deleted = make(map[int]struct{})
			m.cursorStart = -1
			m.cursor = 0
			sort.Slice(m.files, func(i, j int) bool {
				if m.files[i].isDir == m.files[j].isDir {
					return filepath.Ext(m.files[i].name) < filepath.Ext(m.files[j].name)
				}
				return m.files[i].isDir
			})
		case "s":
			m.deleted = make(map[int]struct{})
			m.cursorStart = -1
			m.cursor = 0
			sort.Slice(m.files, func(i, j int) bool {
				return m.files[i].size > m.files[j].size
			})
		case "o":
			m.deleted = make(map[int]struct{})
			m.cursorStart = -1
			m.cursor = 0
			sort.Slice(m.files, func(i, j int) bool {
				return m.files[i].modTime.Before(m.files[j].modTime)
			})
		case "f":
			m.dangerMode = !m.dangerMode
		case "enter", "space":
			if (m.cursorStart == -1) {
				_, ok := m.deleted[m.cursor]
				if ok {
					delete(m.deleted, m.cursor)
				} else {
					m.deleted[m.cursor] = struct{}{}
				}
			} else {
				start := m.cursorStart
				end := m.cursor

				if start > end {
					start, end = end, start
				}

				for i := start; i <= end; i++ {
					_, ok := m.deleted[i]
					if ok {
						delete(m.deleted, i)
					} else {
						m.deleted[i] = struct{}{}
					}
				}
			}
		}
	}

	return m, nil
}

func getViewBox(height, viewstart, fileLen int) (start, end int) {
	reservedSpace := 6
	if (fileLen < height - reservedSpace) {
		return 0, fileLen
	}	

	return viewstart, viewstart + height - reservedSpace
}

func (m model) View() tea.View {
	if m.state == stateConfirm {
		targetIndex := m.deleteQueue[0]
		targetItem := m.files[targetIndex]
		
		promptMessage := fmt.Sprintf("\n  Move '%s' to trash?\n\n  (y) Yes  (n) No  (esc) Cancel", targetItem.name)
		
		confirmationView := tea.NewView(promptMessage)
		confirmationView.AltScreen = true
		return confirmationView
	}

	s := "Mark for deletion\n\n"
	s += green("[t]")+" sort by type, "+green("[s]")+" sort by size, "+green("[o]")+" sort by date created (oldest first), "+red("[f]")+" enter danger mode (no confirmation, no trash)\n\n"
	if (m.height == -1 || m.files == nil || m.cursor == -1) {
		return tea.NewView("Loading...") 
	}	

	start, end := getViewBox(m.height, m.viewstart, len(m.files)) 

	// Iterate over our choices
	for i, choice := range m.files[start:end] {
		cursor := " " // no cursor
		if m.cursorStart != -1 {
			if (m.cursorStart > m.cursor) {
				if (start + i > m.cursor && start + i < m.cursorStart) {
					cursor = "│"
				}	

				if (start + i == m.cursorStart) {
					cursor = "└"
				}
			} else {
				if (start + i >= m.cursorStart && start + i < m.cursor) {
					cursor = "│"
				}

				if (start + i == m.cursorStart) {
					cursor = "┌"
				}
			}	
		}

		if m.cursor == start + i {
			cursor = ">" // cursor!
		}

		checked := " " // not selected
		if _, ok := m.deleted[start + i]; ok {
			checked = "x" // selected!
		}

		// Render the row
		s += fmt.Sprintf("%s [%s] %s\n", cursor, checked, choice.displayStr)
	}

	if m.dangerMode {
		s += "\n\033[41;1;37m DANGER \033[0m Press q to quit. ENTER to select. Shift+X to delete selected items\n"
	} else {
		s += "\nPress q to quit. ENTER to select. Shift+X to delete selected items\n"
	}

	v := tea.NewView(s)
	v.AltScreen = true
	return v
}

func main() {
	p := tea.NewProgram(initialModel())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Alas, there's been an error: %v", err)
		os.Exit(1)
	}
}
