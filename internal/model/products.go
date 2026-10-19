package model

import "meow-launcher/internal/buildinfo"

type ProductStatus string

const (
	StatusAvailable ProductStatus = "available"
	StatusPlanned   ProductStatus = "planned"
)

type Product struct {
	ID          string
	Name        string
	Description string
	Status      ProductStatus
	Version     string
	Channel     string
}

func Catalog() []Product {
	return []Product{
		{
			ID:          "brain2-manager",
			Name:        "Brain2 Manager",
			Description: "本地外接大脑与多 Agent 协作系统。Brain2 的工作区、项目、Agent 与跨产品联动都在独立 Manager 中运行。",
			Status:      StatusAvailable,
			Version:     buildinfo.ManagerDisplayVersion,
			Channel:     "preview",
		},
		{
			ID:          "meow-launcher",
			Name:        "Meow Launcher",
			Description: "Meow 系列产品统一的安装、授权、更新、启动与恢复入口。",
			Status:      StatusAvailable,
			Version:     buildinfo.LauncherDisplayVersion,
			Channel:     "preview",
		},
		{
			ID:          "meowdesk",
			Name:        "Meowdesk",
			Description: "可由用户与 Agent 驱动、组合和扩展的个人工作台。",
			Status:      StatusAvailable,
			Version:     "待发布",
			Channel:     "development",
		},
		{
			ID:          "meow-viewer",
			Name:        "Meow Viewer",
			Description: "Meow 系列内容与成果的统一查看器。当前为产品选择流程占位。",
			Status:      StatusPlanned,
			Version:     "未发布",
			Channel:     "roadmap",
		},
		{
			ID:          "meow-protector",
			Name:        "Meow Protector",
			Description: "面向本地项目的只读检查、风险验证与保护能力。",
			Status:      StatusAvailable,
			Version:     "0.1.0-gate-e",
			Channel:     "preview",
		},
		{
			ID:          "dev-helper",
			Name:        "Dev Helper",
			Description: "面向开发规格、问题证据与人工验收闭环的本地桌面辅助产品。",
			Status:      StatusAvailable,
			Version:     "0.3.0-p1",
			Channel:     "preview",
		},
	}
}
