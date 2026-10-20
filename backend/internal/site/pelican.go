package site

import (
	"context"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/TokenFlux/TokenRouter/internal/pkg/apperror"
)

const (
	PelicanPrompt = `请制作一个“鹈鹕骑车”的 SVG 动画，使用单文件 HTML 展示。自由发挥，重点是创意、画面完成度和骑行动作的自然程度。

每次创作都独立选择以下条件，不要总是使用第一个选项。可以从候选中选择，也可以自行创造新的组合：

【场景】
海边公路、雨夜都市、樱花小巷、沙漠绿洲、雪山山路、热带雨林、欧洲老街、水乡石桥、太空基地、蒸汽工厂、田野乡道、漂浮岛屿、糖果小镇、水下城市。

【服装】
黄色雨衣、复古西装、宇航服、运动骑行服、夏威夷衬衫、侦探风衣、邮差制服、厨师服、朋克夹克、冬季羽绒服、航海水手服、中世纪披风、工装背带裤、度假花裙。

【车子】
城市通勤自行车、公路赛车、山地车、折叠车、复古大前轮自行车、BMX、小轮童车、载货自行车、三轮脚踏车、卧式自行车、蒸汽朋克脚踏车、未来概念脚踏车。可以重新设计车架，但保留清楚、合理的脚踏驱动结构。

【视觉风格】
扁平矢量、复古海报、童书插画、纸片剪贴、像素艺术、水彩质感、赛璐璐动画、黑白漫画、霓虹赛博朋克、包豪斯几何、木刻版画、等距插画、极简线稿、柔和立体卡通。

另外随机决定时间、天气、配色和一个小故事。例如赶着送信、运送一株植物、追逐飞走的帽子、带着面包回家。故事应通过画面和动作表现，避免大段文字说明。

动画要求：
1. 主角具有鲜明的鹈鹕特征：长喙、喉囊、羽翼和蹼足；服装应贴合身体。
2. 两脚始终跟随各自踏板，左右曲柄相差半圈；腿部随踩踏自然屈伸，避免脚与踏板分离。
3. 车轮旋转方向与前进方向一致，链条或传动结构合理；车架保持稳定，不随踏板一起旋转。
4. 鹈鹕身体有轻微起伏，翅膀或手部合理扶住车把；衣摆、羽毛或随车物品有适度动态。
5. 环境具有与场景相符的运动细节，例如雨滴、落叶、云层、灯光或道路掠过；体现远近层次。
6. 动画循环流畅，没有明显跳帧、突然复位、元素穿插或不合理遮挡。
7. 让随机条件形成统一的作品，可以大胆混搭，但不要只更换背景颜色或衣服颜色。

技术与交付：
1. 输出完整、可直接保存并打开的 HTML 文件，以 <!DOCTYPE html> 开始，包含 html、head、body 和闭合标签。页面打开即可展示动画，不要把源码作为文字显示。
2. 使用内联 SVG、CSS，必要时使用少量 JavaScript。所有内容自包含，不依赖外部图片、字体、脚本或网络请求。
3. 页面适配桌面和手机。在 1024×768 的预览窗口内完整展示主体、车轮和播放控件，小屏幕按比例缩放，避免横向溢出。
4. 提供暂停／继续按钮，并支持 prefers-reduced-motion 系统偏好。暂停时同步停止骑行与环境动画；继续时从暂停位置播放。页面不可见时暂停绘制，减少 CPU 占用。
5. 作品将在 sandbox="allow-scripts" 的 iframe 中展示。使用普通 button 和 addEventListener 绑定交互，脚本放在页面元素之后。不要使用 onclick 等内联事件属性、表单、嵌套 iframe、外部模块、浏览器存储或父页面接口。
6. 页面附一个作品名称，以及简短的“创作条件”：场景、服装、车子、风格、天气和故事。
7. 直接输出 HTML，不附加实现说明、Markdown 代码围栏、下载链接或工具调用。`
	PelicanMaxResponseBytes = 1 << 20
	PelicanOutputTokens     = 16384
	PelicanTimeout          = 300 * time.Second
)

var (
	ErrPelicanInvalid  = apperror.BadRequest("PELICAN_INVALID", "Check the group, model, interval and publication consent.")
	ErrPelicanNotFound = apperror.NotFound("PELICAN_NOT_FOUND", "Pelican task or result not found.")
	ErrPelicanBusy     = apperror.Conflict("PELICAN_BUSY", "The task is already queued or running, or this group/model already exists.")
)

type PelicanTask struct {
	ID              int64      `json:"id"`
	GroupID         int64      `json:"group_id"`
	GroupName       string     `json:"group_name"`
	Model           string     `json:"model"`
	Enabled         bool       `json:"enabled"`
	IntervalMinutes int        `json:"interval_minutes"`
	PublicConsent   bool       `json:"public_consent"`
	ManualRequested bool       `json:"manual_requested"`
	NextRunAt       time.Time  `json:"next_run_at"`
	LeaseUntil      *time.Time `json:"lease_until"`
	LastStatus      string     `json:"last_status"`
	LastRunAt       *time.Time `json:"last_run_at"`
}

type PelicanResult struct {
	ID             int64      `json:"id"`
	TaskID         int64      `json:"task_id"`
	GroupID        int64      `json:"group_id"`
	GroupName      string     `json:"group_name"`
	Model          string     `json:"model"`
	Status         string     `json:"status"`
	Published      bool       `json:"published"`
	LatencyMs      int64      `json:"latency_ms"`
	StartedAt      time.Time  `json:"started_at"`
	FinishedAt     *time.Time `json:"finished_at"`
	ProviderID     *int64     `json:"provider_id,omitempty"`
	ExecutionModel string     `json:"execution_model,omitempty"`
	RawResponse    string     `json:"raw_response,omitempty"`
	HTML           string     `json:"html,omitempty"`
	ErrorMessage   string     `json:"error_message,omitempty"`
}

type PelicanFilter struct {
	Page    int
	GroupID int64
	TaskID  int64
	Model   string
	Date    string
	Public  bool
}

type PelicanPage struct {
	Items   []PelicanResult `json:"items"`
	HasMore bool            `json:"has_more"`
	Prompt  string          `json:"prompt"`
}

type PelicanCatalogItem struct {
	GroupID   int64  `json:"group_id"`
	GroupName string `json:"group_name"`
	Model     string `json:"model"`
}

type PelicanClaim struct {
	Task     PelicanTask
	ResultID int64
	Token    string
}

type PelicanExecution struct {
	Text       string
	ProviderID int64
	Model      string
}

type PelicanRepository interface {
	Catalog(context.Context) ([]PelicanCatalogItem, error)
	Tasks(context.Context) ([]PelicanTask, error)
	SaveTask(context.Context, PelicanTask) (PelicanTask, error)
	DeleteTask(context.Context, int64) error
	RequestRun(context.Context, int64) error
	Results(context.Context, PelicanFilter) (PelicanPage, error)
	Result(context.Context, int64, bool) (PelicanResult, error)
	Publish(context.Context, int64, bool) error
	DeleteResult(context.Context, int64) error
	Claim(context.Context) (*PelicanClaim, error)
	Finish(context.Context, PelicanClaim, PelicanResult) error
	Maintain(context.Context) error
}

type PelicanExecutor interface {
	Validate(context.Context, int64, string) error
	Generate(context.Context, PelicanTask) (PelicanExecution, error)
}

type PelicanService struct {
	repo     PelicanRepository
	executor PelicanExecutor
}

func NewPelicanService(repo PelicanRepository, executor PelicanExecutor) *PelicanService {
	return &PelicanService{repo: repo, executor: executor}
}

func (service *PelicanService) Tasks(ctx context.Context) ([]PelicanTask, error) {
	return service.repo.Tasks(ctx)
}

func (service *PelicanService) Catalog(ctx context.Context) ([]PelicanCatalogItem, error) {
	return service.repo.Catalog(ctx)
}

func (service *PelicanService) SaveTask(ctx context.Context, task PelicanTask) (PelicanTask, error) {
	task.Model = strings.TrimSpace(task.Model)
	if task.IntervalMinutes == 0 {
		task.IntervalMinutes = 360
	}
	if task.ID < 0 || task.GroupID <= 0 || task.Model == "" || len(task.Model) > 255 || strings.ContainsAny(task.Model, "*?\r\n") || task.IntervalMinutes < 5 || task.IntervalMinutes > 10080 || (task.Enabled && !task.PublicConsent) {
		return PelicanTask{}, ErrPelicanInvalid
	}
	if task.ID == 0 && task.Enabled {
		return PelicanTask{}, ErrPelicanInvalid
	}
	if err := service.executor.Validate(ctx, task.GroupID, task.Model); err != nil {
		return PelicanTask{}, err
	}
	return service.repo.SaveTask(ctx, task)
}

func (service *PelicanService) DeleteTask(ctx context.Context, id int64) error {
	return service.repo.DeleteTask(ctx, id)
}

func (service *PelicanService) RequestRun(ctx context.Context, id int64) error {
	return service.repo.RequestRun(ctx, id)
}

func (service *PelicanService) Results(ctx context.Context, filter PelicanFilter) (PelicanPage, error) {
	if filter.Page < 1 || filter.Page > 10000 || filter.GroupID < 0 || filter.TaskID < 0 || len(filter.Model) > 255 {
		return PelicanPage{}, ErrPelicanInvalid
	}
	if filter.Date != "" {
		if _, err := time.Parse("2006-01-02", filter.Date); err != nil {
			return PelicanPage{}, ErrPelicanInvalid
		}
	}
	return service.repo.Results(ctx, filter)
}

func (service *PelicanService) Result(ctx context.Context, id int64, public bool) (PelicanResult, error) {
	return service.repo.Result(ctx, id, public)
}

func (service *PelicanService) Publish(ctx context.Context, id int64, published bool) error {
	return service.repo.Publish(ctx, id, published)
}

func (service *PelicanService) DeleteResult(ctx context.Context, id int64) error {
	return service.repo.DeleteResult(ctx, id)
}

func ExtractPelicanHTML(raw string) (string, error) {
	if len(raw) > PelicanMaxResponseBytes || !utf8.ValidString(raw) || strings.ContainsRune(raw, 0) {
		return "", ErrPelicanInvalid
	}
	text := strings.TrimSpace(raw)
	if _, remaining, found := strings.Cut(text, "```"); found {
		if _, content, found := strings.Cut(remaining, "\n"); found {
			if body, _, closed := strings.Cut(content, "```"); closed {
				text = strings.TrimSpace(body)
			}
		}
	}
	lower := strings.ToLower(text)
	for _, tag := range []string{"html", "svg"} {
		start := strings.Index(lower, "<"+tag)
		end := strings.LastIndex(lower, "</"+tag+">")
		if start >= 0 && end < start {
			return "", ErrPelicanInvalid
		}
		if start >= 0 && end > start {
			return text[start : end+len(tag)+3], nil
		}
	}
	return "", ErrPelicanInvalid
}
