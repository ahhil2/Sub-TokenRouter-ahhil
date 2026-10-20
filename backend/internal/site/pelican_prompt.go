package site

import (
	"crypto/rand"
	"strings"
)

func NewPelicanGenerationPrompt() (string, error) {
	var entropy [8]byte
	if _, err := rand.Read(entropy[:]); err != nil {
		return "", err
	}
	return pelicanPromptWithConditions(entropy), nil
}

func pelicanPromptWithConditions(entropy [8]byte) string {
	conditions := []struct {
		label   string
		options []string
	}{
		{"场景", []string{"海边公路", "雨夜都市", "樱花小巷", "沙漠绿洲", "雪山山路", "热带雨林", "欧洲老街", "水乡石桥", "太空基地", "蒸汽工厂", "田野乡道", "漂浮岛屿", "糖果小镇", "水下城市"}},
		{"服装", []string{"黄色雨衣", "复古西装", "宇航服", "运动骑行服", "夏威夷衬衫", "侦探风衣", "邮差制服", "厨师服", "朋克夹克", "冬季羽绒服", "航海水手服", "中世纪披风", "工装背带裤", "度假花裙"}},
		{"车子", []string{"城市通勤自行车", "公路赛车", "山地车", "折叠车", "复古大前轮自行车", "BMX", "小轮童车", "载货自行车", "三轮脚踏车", "卧式自行车", "蒸汽朋克脚踏车", "未来概念脚踏车"}},
		{"视觉风格", []string{"扁平矢量", "复古海报", "童书插画", "纸片剪贴", "像素艺术", "水彩质感", "赛璐璐动画", "黑白漫画", "霓虹赛博朋克", "包豪斯几何", "木刻版画", "等距插画", "极简线稿", "柔和立体卡通"}},
		{"时间", []string{"清晨", "午后", "黄昏", "夜晚"}},
		{"天气", []string{"晴朗微风", "细雨", "薄雾", "飘雪", "多云有风", "雨后初晴"}},
		{"配色", []string{"青绿与暖橙", "靛蓝与金黄", "珊瑚粉与薄荷绿", "奶油白与砖红", "紫罗兰与湖蓝", "黑白与一抹朱红"}},
		{"故事", []string{"赶着送信", "运送一株植物", "追逐飞走的帽子", "带着面包回家", "给朋友送生日蛋糕", "护送一只迷路的小鸟", "带着望远镜去观星", "运送修好的八音盒"}},
	}
	var prompt strings.Builder
	_, _ = prompt.WriteString(PelicanPrompt)
	_, _ = prompt.WriteString("\n\n【本次程序随机抽取的创作条件】\n以下条件已经由服务端独立抽取，请使用这组条件，不要重新选择，也不要复用示例组合。场景与时间、天气有冲突时，保留场景特征并将时间、天气表现为当地或舱外环境。\n")
	for index, condition := range conditions {
		_, _ = prompt.WriteString(condition.label + "：" + condition.options[int(entropy[index])%len(condition.options)] + "\n")
	}
	_, _ = prompt.WriteString("将这组条件落实到画面与动作，并在作品的创作条件中如实列出。仍然直接输出可展示的完整 HTML。")
	return prompt.String()
}
