// Package langs holds the language table shared by the picker, the services
// and the UI. Codes follow the BCP-47-ish form the upstream APIs expect.
package langs

// Auto is the source-language sentinel meaning "detect".
const Auto = "auto"

type Lang struct {
	Code string `json:"code"`
	Name string `json:"name"`
}

var table = []Lang{
	{Auto, "自动检测"},
	{"zh-CN", "简体中文"}, {"zh-TW", "繁体中文"}, {"en", "英语"}, {"ja", "日语"},
	{"ko", "韩语"}, {"fr", "法语"}, {"de", "德语"}, {"es", "西班牙语"}, {"ru", "俄语"},
	{"it", "意大利语"}, {"pt", "葡萄牙语"}, {"th", "泰语"}, {"vi", "越南语"},
	{"ar", "阿拉伯语"}, {"nl", "荷兰语"}, {"pl", "波兰语"}, {"tr", "土耳其语"},
	{"fa", "波斯语"}, {"hi", "印地语"}, {"id", "印尼语"}, {"ms", "马来语"},
	{"sv", "瑞典语"}, {"no", "挪威语"}, {"da", "丹麦语"}, {"fi", "芬兰语"},
	{"cs", "捷克语"}, {"sk", "斯洛伐克语"}, {"hu", "匈牙利语"}, {"ro", "罗马尼亚语"},
	{"bg", "保加利亚语"}, {"el", "希腊语"}, {"uk", "乌克兰语"}, {"he", "希伯来语"},
	{"ca", "加泰罗尼亚语"}, {"et", "爱沙尼亚语"}, {"lv", "拉脱维亚语"},
	{"lt", "立陶宛语"}, {"sl", "斯洛文尼亚语"}, {"hr", "克罗地亚语"},
	{"sr", "塞尔维亚语"}, {"mk", "马其顿语"}, {"sq", "阿尔巴尼亚语"},
	{"hy", "亚美尼亚语"}, {"az", "阿塞拜疆语"}, {"be", "白俄罗斯语"},
	{"ka", "格鲁吉亚语"}, {"kk", "哈萨克语"}, {"ky", "吉尔吉斯语"}, {"lo", "老挝语"},
	{"my", "缅甸语"}, {"ne", "尼泊尔语"}, {"mn", "蒙古语"}, {"tl", "菲律宾语"},
}

func All() []Lang { return table }

// Targets is All without the detection sentinel, for target-language pickers.
func Targets() []Lang {
	out := make([]Lang, 0, len(table)-1)
	for _, l := range table {
		if l.Code != Auto {
			out = append(out, l)
		}
	}
	return out
}

// Name maps a code back to its display name, falling back to the code so an
// unknown service-specific locale still renders readably.
func Name(code string) string {
	for _, l := range table {
		if l.Code == code {
			return l.Name
		}
	}
	return code
}
