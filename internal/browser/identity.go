package browser

import (
	"fmt"
	"math/rand"
	"sort"
)

// ──────────────────── Chrome 多版本支持 ────────────────────

type chromeVersion struct {
	Version string
	SecUA   string
}

// genChromeVersion 动态生成 Chrome 版本信息
func genChromeVersion() chromeVersion {
	// 只选择 tls-client 当前内置桌面 Chrome profile 支持的主版本，避免 UA/sec-ch-ua 与 TLS/HTTP2 指纹错配。
	versions := []string{"120", "124", "131", "133", "144"}
	v := versions[rand.Intn(len(versions))]

	greaseBrands := []string{"Not_A Brand", "Not(A:Brand", "Not-A.Brand", "Not)A;Brand", "Not/A)Brand", "Not A;Brand", "Not?A_Brand"}
	greaseBrand := greaseBrands[rand.Intn(len(greaseBrands))]
	greaseVer := fmt.Sprintf("%d", 8+rand.Intn(92)) // 8~99

	secUA := fmt.Sprintf(`"%s";v="%s", "Chromium";v="%s", "Google Chrome";v="%s"`, greaseBrand, greaseVer, v, v)
	return chromeVersion{
		Version: v + ".0.0.0",
		SecUA:   secUA,
	}
}

// ──────────────────── lsUbid 前缀池 ────────────────────

var lsubidPrefixes = []string{"X10", "X19", "X42", "X55", "X73", "X81", "X96"}

// ──────────────────── WebGL 扩展 ────────────────────

var webglExtCore = []string{
	"ANGLE_instanced_arrays", "EXT_blend_minmax", "EXT_color_buffer_half_float",
	"EXT_float_blend", "EXT_frag_depth", "EXT_shader_texture_lod",
	"EXT_texture_filter_anisotropic", "EXT_sRGB", "KHR_parallel_shader_compile",
	"OES_element_index_uint", "OES_fbo_render_mipmap", "OES_standard_derivatives",
	"OES_texture_float", "OES_texture_float_linear", "OES_texture_half_float",
	"OES_texture_half_float_linear", "OES_vertex_array_object",
	"WEBGL_color_buffer_float", "WEBGL_compressed_texture_s3tc",
	"WEBGL_compressed_texture_s3tc_srgb", "WEBGL_debug_renderer_info",
	"WEBGL_debug_shaders", "WEBGL_depth_texture", "WEBGL_draw_buffers",
	"WEBGL_lose_context", "WEBGL_multi_draw",
}

var webglExtOptional = []string{
	"EXT_disjoint_timer_query", "EXT_texture_compression_bptc",
	"EXT_texture_compression_rgtc", "WEBGL_compressed_texture_astc",
	"WEBGL_compressed_texture_etc", "OES_draw_buffers_indexed",
	"EXT_color_buffer_float",
}

// ──────────────────── 插件 (Chrome 固有) ────────────────────

var pluginsPool = []map[string]string{
	{"name": "PDF Viewer", "filename": "internal-pdf-viewer", "description": "Portable Document Format"},
	{"name": "Chrome PDF Viewer", "filename": "internal-pdf-viewer", "description": "Portable Document Format"},
	{"name": "Chromium PDF Viewer", "filename": "internal-pdf-viewer", "description": "Portable Document Format"},
	{"name": "Microsoft Edge PDF Viewer", "filename": "internal-pdf-viewer", "description": "Portable Document Format"},
	{"name": "WebKit built-in PDF", "filename": "internal-pdf-viewer", "description": "Portable Document Format"},
}

// ──────────────────── 数据结构 ────────────────────

// ScreenInfo 屏幕信息
type ScreenInfo struct {
	Width, Height, AvailWidth, AvailHeight, ColorDepth int
}

// BrowserIdentity 浏览器身份
type BrowserIdentity struct {
	ChromeVer           string
	UA                  string
	SecUA               string
	GPUVendor           string
	GPUModel            string
	WebGLExts           []string
	CanvasHash          int32
	HistogramBase       [256]int
	MathTan             string
	MathSin             string
	MathCos             string
	Plugins             []map[string]string
	Screen              ScreenInfo
	DeviceMemory        int
	HardwareConcurrency int
	Platform            string
	LsubidPrefixSignin  string
	LsubidPrefixProfile string
	WebpackHash         string
}

// ──────────────────── 算法: GPU 配置生成 ────────────────────
// 规律: Vendor = "Google Inc. ({芯片厂商})"
//       Model  = "ANGLE ({芯片厂商}, {芯片型号} Direct3D11 vs_5_0 ps_5_0, D3D11)"

func genGPU() (vendor, model string) {
	type gpuFamily struct {
		chipVendor string
		prefix     string
		models     []string
	}

	families := []gpuFamily{
		{
			chipVendor: "Intel",
			prefix:     "Intel(R) ",
			models: []string{
				"UHD Graphics 630", "UHD Graphics 730", "UHD Graphics 770",
				"HD Graphics 530", "HD Graphics 620", "HD Graphics 630",
				"Iris(R) Xe Graphics", "Iris(R) Xe Graphics (0x000046A6)",
				"Iris(R) Plus Graphics", "UHD Graphics", "HD Graphics 520",
				"UHD Graphics 620", "Iris(R) Plus Graphics 655", "Iris(R) Plus Graphics 640",
				"HD Graphics 4600", "HD Graphics 5500",
			},
		},
		{
			chipVendor: "NVIDIA",
			prefix:     "NVIDIA ",
			models: []string{
				"GeForce GTX 960", "GeForce GTX 970", "GeForce GTX 980 Ti",
				"GeForce GTX 1050 Ti", "GeForce GTX 1060 6GB", "GeForce GTX 1070",
				"GeForce GTX 1080", "GeForce GTX 1080 Ti", "GeForce GTX 1650",
				"GeForce GTX 1660 Super", "GeForce RTX 2060", "GeForce RTX 2070",
				"GeForce RTX 2080", "GeForce RTX 3050 Laptop GPU", "GeForce RTX 3060",
				"GeForce RTX 3060 Ti", "GeForce RTX 3070", "GeForce RTX 3080",
				"GeForce RTX 4060", "GeForce RTX 4070", "GeForce RTX 4080", "GeForce RTX 4090",
			},
		},
		{
			chipVendor: "AMD",
			prefix:     "AMD ",
			models: []string{
				"Radeon RX 580", "Radeon RX 5600 XT", "Radeon RX 5700 XT",
				"Radeon RX 6600 XT", "Radeon RX 6700 XT", "Radeon RX 6800 XT",
				"Radeon RX 7600", "Radeon RX 7800 XT", "Radeon RX 7900 XTX",
				"Radeon Vega 8 Graphics", "Radeon(TM) Graphics", "Radeon RX Vega 11 Graphics",
				"Radeon RX 5500 XT", "Radeon R9 390", "Radeon RX 480",
			},
		},
	}

	f := families[rand.Intn(len(families))]
	m := f.models[rand.Intn(len(f.models))]

	vendor = fmt.Sprintf("Google Inc. (%s)", f.chipVendor)
	model = fmt.Sprintf("ANGLE (%s, %s%s Direct3D11 vs_5_0 ps_5_0, D3D11)", f.chipVendor, f.prefix, m)
	return
}

// ──────────────────── 算法: 屏幕分辨率生成 ────────────────────
// 规律: AvailHeight = Height - taskbar(32~48), AvailWidth = Width, ColorDepth = 24

func genScreen() ScreenInfo {
	type resolution struct {
		w, h int
	}

	// 按宽高比分组的常见分辨率
	r16x9 := []resolution{
		{1366, 768}, {1536, 864}, {1600, 900},
		{1920, 1080}, {2560, 1440}, {3840, 2160},
	}
	r16x10 := []resolution{
		{1440, 900}, {1680, 1050}, {1920, 1200}, {2560, 1600},
	}
	r21x9 := []resolution{
		{2560, 1080}, {3440, 1440},
	}
	rOther := []resolution{
		{1280, 720}, {1360, 768}, {2880, 1800},
	}

	// 按市场份额加权: 16:9 最常见
	pools := [][]resolution{r16x9, r16x9, r16x9, r16x10, r21x9, rOther}
	pool := pools[rand.Intn(len(pools))]
	res := pool[rand.Intn(len(pool))]

	// 任务栏高度 32~48 像素
	taskbar := 32 + rand.Intn(17) // 32-48
	// 圆整到 8 的倍数 (Windows 常见)
	taskbar = (taskbar / 8) * 8
	if taskbar < 32 {
		taskbar = 32
	}

	colorDepths := []int{24, 24, 24, 24, 30} // 24常见, 30是 HDR

	return ScreenInfo{
		Width:       res.w,
		Height:      res.h,
		AvailWidth:  res.w,
		AvailHeight: res.h - taskbar,
		ColorDepth:  colorDepths[rand.Intn(len(colorDepths))],
	}
}

// ──────────────────── 算法: Math 精度生成 ────────────────────
// 规律: Math.tan/sin/cos(-1e300) 在不同硬件上仅末位 1-2 位有差异
// tan 基准: "-1.4214488238747245"  末位 3~7
// sin 基准: "0.8178819121159085"   末位 3~7
// cos 有两个家族:
//   家族A: "-0.5753861119575491"   末位 89~93
//   家族B: "-0.5765775004286854"   末位 53~55

func genMath() (tan, sin, cos string) {
	tan = []string{
		"-1.4214488238747245",
		"-1.4214488238747243",
		"-1.4214488238747247",
	}[rand.Intn(3)]
	sin = []string{
		"0.8178819121159085",
		"0.8178819121159083",
		"0.8178819121159087",
	}[rand.Intn(3)]
	// cos 有两个家族, 家族A 更常见 (~70%)
	if rand.Intn(10) < 7 {
		cos = []string{
			"-0.5753861119575491",
			"-0.5753861119575489",
			"-0.5753861119575493",
		}[rand.Intn(3)]
	} else {
		cos = []string{
			"-0.5765775004286854",
			"-0.5765775004286853",
			"-0.5765775004286855",
		}[rand.Intn(3)]
	}
	return
}

// ──────────────────── 算法: Canvas Histogram 模拟 ────────────────────
// 真实 FWCIM 使用 150×60 canvas 绘制固定图形、字体和混合模式后，
// 对 getImageData(...).data 做 256 桶直方图。这个分布主要由 Chrome/Windows
// 2D canvas 渲染决定，同一桌面 Chrome 家族非常稳定；TES 会校验它是否像真实画布。
//
// 下方基线来自本地 Chromium 运行真实 profile FWCIM collector 的明文捕获。

var chrome2DCanvasHistogramBase = [256]int{
	12839, 66, 58, 54, 46, 73, 29, 41, 36, 24, 94, 46, 32, 37, 23, 80,
	53, 29, 62, 123, 29, 29, 31, 50, 30, 28, 36, 24, 33, 30, 36, 39,
	39, 51, 23, 22, 43, 142, 25, 40, 29, 37, 26, 30, 36, 31, 20, 38,
	37, 38, 43, 35, 32, 122, 18, 40, 25, 30, 19, 17, 59, 41, 15, 17,
	23, 19, 42, 20, 26, 44, 14, 21, 29, 15, 43, 77, 17, 14, 50, 47,
	32, 24, 22, 23, 33, 21, 17, 31, 33, 62, 30, 27, 23, 28, 125, 20,
	38, 13, 28, 90, 84, 26, 515, 37, 13, 45, 91, 25, 13, 16, 21, 46,
	47, 12, 18, 15, 21, 95, 24, 22, 29, 40, 21, 67, 44, 37, 29, 128,
	190, 47, 24, 54, 37, 14, 24, 26, 22, 41, 135, 18, 23, 15, 20, 29,
	21, 19, 30, 21, 11, 67, 55, 20, 20, 86, 15, 56, 24, 21, 13, 28,
	16, 70, 11, 9, 27, 35, 9, 15, 18, 25, 22, 30, 24, 41, 19, 36,
	57, 82, 15, 25, 69, 54, 12, 28, 12, 8, 19, 11, 18, 52, 16, 29,
	14, 5, 31, 17, 18, 29, 59, 15, 29, 40, 107, 41, 56, 75, 100, 12,
	39, 20, 24, 14, 21, 42, 43, 24, 25, 21, 70, 36, 35, 26, 69, 83,
	22, 59, 31, 43, 64, 44, 25, 37, 22, 41, 28, 46, 108, 36, 30, 74,
	62, 41, 62, 34, 83, 107, 39, 81, 45, 32, 89, 60, 50, 55, 89, 12719,
}

func generateCanvasData() (int32, [256]int) {
	return -2120415875, chrome2DCanvasHistogramBase
}

// abs 整数绝对值
func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// ──────────────────── 核心: 随机身份生成 ────────────────────

// RandomIdentity 创建随机浏览器身份
func RandomIdentity() *BrowserIdentity {
	// Chrome 版本
	cv := genChromeVersion()

	// GPU
	gpuVendor, gpuModel := genGPU()

	// Screen
	screen := genScreen()

	// 硬件参数
	memories := []int{2, 4, 6, 8, 12, 16, 24, 32, 64}
	deviceMemory := memories[rand.Intn(len(memories))]

	concurrencies := []int{2, 4, 6, 8, 10, 12, 14, 16, 20, 24, 32}
	hardwareConcurrency := concurrencies[rand.Intn(len(concurrencies))]

	platform := "Win32"

	// Math 精度
	mathTan, mathSin, mathCos := genMath()

	// Canvas 数据
	canvasHash, histogram := generateCanvasData()

	// WebGL 扩展
	exts := make([]string, len(webglExtCore))
	copy(exts, webglExtCore)
	nOpt := rand.Intn(5)
	if nOpt > 0 && nOpt <= len(webglExtOptional) {
		perm := rand.Perm(len(webglExtOptional))
		for i := 0; i < nOpt; i++ {
			exts = append(exts, webglExtOptional[perm[i]])
		}
	}
	sort.Strings(exts)

	// 插件 (Chrome 内置 PDF 插件, 随机排列)
	plugins := make([]map[string]string, len(pluginsPool))
	copy(plugins, pluginsPool)
	rand.Shuffle(len(plugins), func(i, j int) { plugins[i], plugins[j] = plugins[j], plugins[i] })

	ua := fmt.Sprintf("Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/%s Safari/537.36", cv.Version)

	return &BrowserIdentity{
		ChromeVer:           cv.Version,
		UA:                  ua,
		SecUA:               cv.SecUA,
		GPUVendor:           gpuVendor,
		GPUModel:            gpuModel,
		WebGLExts:           exts,
		CanvasHash:          canvasHash,
		HistogramBase:       histogram,
		MathTan:             mathTan,
		MathSin:             mathSin,
		MathCos:             mathCos,
		Plugins:             plugins,
		Screen:              screen,
		DeviceMemory:        deviceMemory,
		HardwareConcurrency: hardwareConcurrency,
		Platform:            platform,
		LsubidPrefixSignin:  lsubidPrefixes[rand.Intn(len(lsubidPrefixes))],
		LsubidPrefixProfile: lsubidPrefixes[rand.Intn(len(lsubidPrefixes))],
		WebpackHash:         fmt.Sprintf("%x", rand.Int63())[:10],
	}
}
