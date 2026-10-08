// Package site 管理站点展示、公告、页面权限和站长私信。
//
// 公告从 service.go 和 announcement.go 读取，展示设置从 public.go 和
// display_settings.go 读取，私信从 messages.go 读取。
// navigation.go 和 pages.go 定义菜单及页面权限。
// admin_settings*.go 与 settings_participant.go 处理设置更新。
// httpapi、postgres 和 filesystem 子包分别负责 HTTP、数据库和文件读取。
package site
