//go:build windows

package ui

import (
	"fmt"
	"net/url"
	"path/filepath"
	"runtime"
	"syscall"
	"unsafe"

	"meow-launcher/internal/brain2"
	"meow-launcher/internal/model"
	"meow-launcher/internal/settings"
)

const className = "MeowLauncherV043Window"

const (
	idLogin         = 1001
	idSkip          = 1002
	idBackWelcome   = 1003
	idProduct       = 1004
	idEnterProduct  = 1005
	idBackProducts  = 1100
	idPath          = 1101
	idChoosePath    = 1102
	idInstall       = 1103
	idAttachBrain   = 1104
	idOpenFolder    = 1105
	idOpenObsidian  = 1106
	idRuntime       = 1107
	idHealth        = 1108
	idProjectName   = 1109
	idCreateProject = 1110
	idAttachProject = 1111
)

type App struct {
	hwnd         HWND
	instance     HINSTANCE
	catalog      []model.Product
	controls     map[int]HWND
	welcome      []HWND
	products     []HWND
	brain2       []HWND
	selectedRoot string
}

var active *App

func Run() error {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pCoInitializeEx.Call(0, COINIT_APARTMENTTHREADED)
	defer pCoUninitialize.Call()

	h, _, _ := pGetModuleHandleW.Call(0)
	inst := HINSTANCE(h)
	cursor, _, _ := pLoadCursorW.Call(0, IDC_ARROW)
	wc := WNDCLASSEX{CbSize: uint32(unsafe.Sizeof(WNDCLASSEX{})), Style: CS_HREDRAW | CS_VREDRAW, LpfnWndProc: syscall.NewCallback(wndProc), HInstance: inst, HCursor: HCURSOR(cursor), HbrBackground: HBRUSH(COLOR_WINDOW + 1), LpszClassName: utf16Ptr(className)}
	if r, _, err := pRegisterClassExW.Call(uintptr(unsafe.Pointer(&wc))); r == 0 {
		return fmt.Errorf("RegisterClassExW failed: %v", err)
	}

	app := &App{instance: inst, catalog: model.Catalog(), controls: map[int]HWND{}}
	active = app
	app.hwnd = createWindow(0, className, "Meow Launcher — Preview v0.4.3", WS_OVERLAPPEDWINDOW|WS_VISIBLE, 100, 80, 980, 700, 0, 0, inst)
	if app.hwnd == 0 {
		return fmt.Errorf("CreateWindowExW failed")
	}
	app.build()
	app.restoreRememberedRoot()
	app.showWelcome()
	pShowWindow.Call(uintptr(app.hwnd), SW_SHOW)
	pUpdateWindow.Call(uintptr(app.hwnd))
	var m MSG
	for {
		r, _, _ := pGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(r) <= 0 {
			break
		}
		pTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		pDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
	return nil
}

func (a *App) add(id int, class, text string, style uint32, x, y, w, h int32, parent HWND) HWND {
	c := createWindow(0, class, text, style|WS_CHILD|WS_VISIBLE, x, y, w, h, parent, int32(id), a.instance)
	a.controls[id] = c
	return c
}
func (a *App) label(text string, x, y, w, h int32) HWND {
	return a.add(0, "STATIC", text, SS_LEFT, x, y, w, h, a.hwnd)
}
func (a *App) button(id int, text string, x, y, w, h int32) HWND {
	return a.add(id, "BUTTON", text, WS_TABSTOP|BS_PUSHBUTTON, x, y, w, h, a.hwnd)
}
func (a *App) build() {
	brand := a.label("Meow Launcher", 34, 20, 420, 45)
	version := a.label("Preview v0.4.3", 37, 62, 160, 24)
	_ = brand
	_ = version
	w1 := a.label("欢迎使用 Meow Launcher", 70, 130, 700, 48)
	w2 := a.label("一个入口，安装、启动、升级和维护你的 Meow 系列产品。", 72, 184, 760, 30)
	w3 := a.label("登录功能将在后续版本接入。当前可先跳过进入体验。", 72, 252, 760, 30)
	login := a.button(idLogin, "登录（尚未接入）", 72, 305, 360, 48)
	skip := a.button(idSkip, "跳过，进入体验", 450, 305, 360, 48)
	a.welcome = []HWND{w1, w2, w3, login, skip}
	p1 := a.label("选择产品", 70, 130, 400, 42)
	p2 := a.label("选择一个产品后，进入它自己的安装、启动和维护页面。", 72, 180, 760, 28)
	combo := a.add(idProduct, "COMBOBOX", "", WS_TABSTOP|CBS_DROPDOWNLIST|WS_VSCROLL, 72, 235, 520, 300, a.hwnd)
	for _, p := range a.catalog {
		pSendMessageW.Call(uintptr(combo), CB_ADDSTRING, 0, uintptr(unsafe.Pointer(utf16Ptr(p.Name))))
	}
	pSendMessageW.Call(uintptr(combo), CB_SETCURSEL, 0, 0)
	desc := a.label(a.catalog[0].Description, 72, 300, 760, 70)
	a.controls[2001] = desc
	status := a.label("状态：可用", 72, 385, 300, 28)
	a.controls[2002] = status
	enter := a.button(idEnterProduct, "进入 Brain2", 72, 435, 760, 48)
	back := a.button(idBackWelcome, "返回欢迎页", 72, 515, 180, 40)
	a.products = []HWND{p1, p2, combo, desc, status, enter, back}
	b0 := a.button(idBackProducts, "← 选择其他产品", 52, 112, 180, 36)
	b1 := a.label("Brain2", 52, 165, 300, 42)
	b2 := a.label("安装、接入、打开和维护你的本地外接大脑。", 52, 210, 760, 28)
	path := a.add(idPath, "EDIT", "", WS_BORDER, 52, 260, 650, 32, a.hwnd)
	choose := a.button(idChoosePath, "选择目录", 720, 258, 150, 36)
	state := a.label("状态：尚未选择 Brain2 目录", 52, 310, 760, 28)
	a.controls[2101] = state
	install := a.button(idInstall, "新安装 Brain2", 52, 355, 180, 42)
	attach := a.button(idAttachBrain, "接入已有 Brain2", 247, 355, 180, 42)
	open := a.button(idOpenFolder, "打开文件夹", 442, 355, 145, 42)
	obs := a.button(idOpenObsidian, "在 Obsidian 中打开", 602, 355, 268, 42)
	runtimeBtn := a.button(idRuntime, "生成 Agent Runtime", 52, 420, 220, 42)
	health := a.button(idHealth, "健康检查", 287, 420, 170, 42)
	projectLabel := a.label("第一个项目（填写名称后创建，或挂载已有目录）", 52, 478, 560, 26)
	projectName := a.add(idProjectName, "EDIT", "", WS_BORDER|WS_TABSTOP, 52, 510, 350, 34, a.hwnd)
	createProject := a.button(idCreateProject, "创建内部项目", 417, 508, 180, 38)
	attachProject := a.button(idAttachProject, "挂载外部项目", 612, 508, 258, 38)
	statusLine := a.label("等待操作", 52, 575, 818, 30)
	a.controls[2102] = statusLine
	a.brain2 = []HWND{b0, b1, b2, path, choose, state, install, attach, open, obs, runtimeBtn, health, projectLabel, projectName, createProject, attachProject, statusLine}
}
func (a *App) showPage(list []HWND) {
	for _, c := range a.welcome {
		show(c, false)
	}
	for _, c := range a.products {
		show(c, false)
	}
	for _, c := range a.brain2 {
		show(c, false)
	}
	for _, c := range list {
		show(c, true)
	}
}
func (a *App) showWelcome()  { a.showPage(a.welcome) }
func (a *App) showProducts() { a.showPage(a.products); a.updateProduct() }
func (a *App) showBrain2()   { a.showPage(a.brain2); a.refreshBrain2State() }
func (a *App) updateProduct() {
	combo := a.controls[idProduct]
	idx, _, _ := pSendMessageW.Call(uintptr(combo), CB_GETCURSEL, 0, 0)
	if int(idx) < 0 || int(idx) >= len(a.catalog) {
		return
	}
	p := a.catalog[int(idx)]
	setText(a.controls[2001], p.Description)
	if p.Status == model.StatusAvailable {
		setText(a.controls[2002], "状态：可用")
		setText(a.controls[idEnterProduct], "进入 "+p.Name)
		enable(a.controls[idEnterProduct], true)
	} else {
		setText(a.controls[2002], "状态：规划中")
		setText(a.controls[idEnterProduct], "查看占位状态")
		enable(a.controls[idEnterProduct], true)
	}
}
func (a *App) selectedProduct() model.Product {
	idx, _, _ := pSendMessageW.Call(uintptr(a.controls[idProduct]), CB_GETCURSEL, 0, 0)
	if int(idx) < 0 || int(idx) >= len(a.catalog) {
		return a.catalog[0]
	}
	return a.catalog[int(idx)]
}
func (a *App) refreshBrain2State() {
	if a.selectedRoot == "" {
		setText(a.controls[2101], "状态：尚未选择 Brain2 目录")
		return
	}
	if brain2.IsWorkspace(a.selectedRoot) {
		setText(a.controls[2101], "状态：已接入 Brain2")
	} else {
		setText(a.controls[2101], "状态：目录已选择，尚未安装 Brain2")
	}
}
func (a *App) chooseFolder() string { return browseFolder(a.hwnd) }
func (a *App) setRoot(path string) {
	a.selectedRoot = filepath.Clean(path)
	setText(a.controls[idPath], path)
	a.refreshBrain2State()
}

func (a *App) restoreRememberedRoot() {
	state, err := settings.Load()
	if err == nil && brain2.IsWorkspace(state.Brain2Root) {
		a.setRoot(state.Brain2Root)
	}
}

func (a *App) rememberRoot() error {
	return settings.Save(a.selectedRoot)
}
func (a *App) onCommand(id uint16, code uint16) {
	switch int(id) {
	case idLogin:
		msgBox(a.hwnd, "登录功能将在后续版本接入。\n\n当前请点击“跳过，进入体验”。", "Meow Launcher", MB_OK|MB_ICONINFORMATION)
	case idSkip:
		a.showProducts()
	case idBackWelcome:
		a.showWelcome()
	case idProduct:
		if code == 1 {
			a.updateProduct()
		}
	case idEnterProduct:
		p := a.selectedProduct()
		if p.ID == "brain2" {
			a.showBrain2()
		} else {
			msgBox(a.hwnd, p.Name+" 当前处于规划中。", "Meow Launcher", MB_OK|MB_ICONINFORMATION)
		}
	case idBackProducts:
		a.showProducts()
	case idChoosePath:
		if p := a.chooseFolder(); p != "" {
			a.setRoot(p)
		}
	case idInstall:
		if a.selectedRoot == "" {
			msgBox(a.hwnd, "请先选择一个新的空目录。", "Meow Launcher", MB_OK|MB_ICONWARNING)
			return
		}
		if err := brain2.Initialize(a.selectedRoot); err != nil {
			msgBox(a.hwnd, err.Error(), "安装失败", MB_OK|MB_ICONERROR)
		} else {
			a.refreshBrain2State()
			if err := a.rememberRoot(); err != nil {
				setText(a.controls[2102], "Brain2 已安装，但记住目录失败："+err.Error())
			} else {
				setText(a.controls[2102], "Brain2 安装完成，目录已记住")
			}
			msgBox(a.hwnd, "Brain2 已安装。", "Meow Launcher", MB_OK|MB_ICONINFORMATION)
		}
	case idAttachBrain:
		if p := a.chooseFolder(); p != "" {
			if !brain2.IsWorkspace(p) {
				msgBox(a.hwnd, "没有找到 .brain2\\config.json。", "Meow Launcher", MB_OK|MB_ICONWARNING)
			} else {
				a.setRoot(p)
				if err := a.rememberRoot(); err != nil {
					setText(a.controls[2102], "Brain2 已接入，但记住目录失败："+err.Error())
				} else {
					setText(a.controls[2102], "已接入已有 Brain2，目录已记住")
				}
			}
		}
	case idOpenFolder:
		if a.selectedRoot != "" {
			shellOpen(a.selectedRoot)
		}
	case idOpenObsidian:
		if a.selectedRoot != "" {
			shellOpen("obsidian://open?path=" + url.QueryEscape(a.selectedRoot))
		}
	case idRuntime:
		if err := brain2.PrepareRuntime(a.selectedRoot, "codex"); err != nil {
			msgBox(a.hwnd, err.Error(), "Runtime 失败", MB_OK|MB_ICONERROR)
		} else {
			setText(a.controls[2102], "Agent Runtime 已生成")
		}
	case idCreateProject:
		name := getText(a.controls[idProjectName])
		if err := brain2.CreateInternalProject(a.selectedRoot, name, ""); err != nil {
			msgBox(a.hwnd, err.Error(), "创建项目失败", MB_OK|MB_ICONERROR)
		} else {
			setText(a.controls[2102], "内部项目已创建："+name)
		}
	case idAttachProject:
		source := a.chooseFolder()
		if source == "" {
			return
		}
		name := getText(a.controls[idProjectName])
		if name == "" {
			name = filepath.Base(source)
			setText(a.controls[idProjectName], name)
		}
		if err := brain2.AttachExternalProject(a.selectedRoot, name, source, ""); err != nil {
			msgBox(a.hwnd, err.Error(), "挂载项目失败", MB_OK|MB_ICONERROR)
		} else {
			setText(a.controls[2102], "外部项目已挂载（原目录未移动）："+name)
		}
	case idHealth:
		r, err := brain2.Check(a.selectedRoot)
		if err != nil {
			msgBox(a.hwnd, err.Error(), "健康检查失败", MB_OK|MB_ICONERROR)
		} else {
			setText(a.controls[2102], fmt.Sprintf("健康检查：%s，得分 %d", r.OverallStatus, r.Score))
			msgBox(a.hwnd, fmt.Sprintf("总体状态：%s\n得分：%d", r.OverallStatus, r.Score), "健康检查", MB_OK|MB_ICONINFORMATION)
		}
	}
}

func wndProc(hwnd HWND, msg uint32, w WPARAM, l LPARAM) LRESULT {
	if active != nil {
		switch msg {
		case WM_COMMAND:
			active.onCommand(loword(uintptr(w)), hiword(uintptr(w)))
			return 0
		case WM_DESTROY:
			pPostQuitMessage.Call(0)
			return 0
		}
	}
	r, _, _ := pDefWindowProcW.Call(uintptr(hwnd), uintptr(msg), uintptr(w), uintptr(l))
	return LRESULT(r)
}

func browseFolder(owner HWND) string {
	type browseInfo struct {
		owner       HWND
		root        uintptr
		displayName *uint16
		title       *uint16
		flags       uint32
		callback    uintptr
		lParam      LPARAM
		image       int32
	}
	buf := make([]uint16, 260)
	bi := browseInfo{owner: owner, displayName: &buf[0], title: utf16Ptr("选择目录"), flags: 0x00000001 | 0x00000040}
	proc := shell32.NewProc("SHBrowseForFolderW")
	getPath := shell32.NewProc("SHGetPathFromIDListW")
	free := ole32.NewProc("CoTaskMemFree")
	pidl, _, _ := proc.Call(uintptr(unsafe.Pointer(&bi)))
	if pidl == 0 {
		return ""
	}
	defer free.Call(pidl)
	path := make([]uint16, 32768)
	ok, _, _ := getPath.Call(pidl, uintptr(unsafe.Pointer(&path[0])))
	if ok == 0 {
		return ""
	}
	return syscall.UTF16ToString(path)
}
func shellOpen(target string) {
	verb := utf16Ptr("open")
	pShellExecuteW.Call(0, uintptr(unsafe.Pointer(verb)), uintptr(unsafe.Pointer(utf16Ptr(target))), 0, 0, SW_SHOW)
}
