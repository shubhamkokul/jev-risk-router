package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"io"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/joho/godotenv"
)

// --- Domain Models for Risk Router ---
type Transaction struct {
	AccountAgeDays          int     `json:"account_age_days"`
	DeviceFingerprint       string  `json:"device_fingerprint"`
	IPDistanceFromBillingKm int     `json:"ip_distance_from_billing_km"`
	IsVPNOrProxy            bool    `json:"is_vpn_or_proxy"`
	Velocity1Hr             int     `json:"velocity_1hr"`
	MerchantCategory        string  `json:"merchant_category"`
	AmountUSD               float64 `json:"amount_usd"`
	HistoricalAvgAmountUSD  float64 `json:"historical_avg_amount_usd"`
}

type Profile struct {
	Name string
	Tx   Transaction
}

func getProfiles() []Profile {
	return []Profile{
		{"Alice (Good User)", Transaction{1200, "iphone_15_pro_trusted", 2, false, 1, "Groceries", 6.50, 5.00}},
		{"Bob (Good User)", Transaction{500, "macbook_pro_trusted", 10, false, 1, "Travel", 800.00, 600.00}},
		{"Carol (Good User)", Transaction{2000, "pixel_7_trusted", 6000, false, 2, "Hotel", 250.00, 100.00}},
		{"Eve (ATO Risk)", Transaction{1500, "unknown_android_emulator", 300, true, 3, "Electronics", 1200.00, 50.00}},
		{"Mallory (Carding Risk)", Transaction{1, "windows_pc_unknown", 8000, true, 1, "Digital Gift Cards", 500.00, 0.00}},
		{"Trent (Test Auth)", Transaction{10, "iphone_8_unknown", 50, false, 15, "Charity", 1.00, 0.00}},
	}
}

func generateSynthetic() Profile {
	isFraud := rand.Float32() > 0.5
	categories := []string{"Groceries", "Digital Goods", "Crypto", "Electronics", "Travel", "Charity"}
	cat := categories[rand.Intn(len(categories))]
	
	if isFraud {
		return Profile{"Synthetic User (Likely Fraud)", Transaction{
			AccountAgeDays: rand.Intn(30),
			DeviceFingerprint: "unknown_device_" + fmt.Sprint(rand.Intn(1000)),
			IPDistanceFromBillingKm: rand.Intn(10000) + 1000,
			IsVPNOrProxy: true,
			Velocity1Hr: rand.Intn(10) + 2,
			MerchantCategory: cat,
			AmountUSD: float64(rand.Intn(2000)) + rand.Float64(),
			HistoricalAvgAmountUSD: float64(rand.Intn(50)),
		}}
	} else {
		return Profile{"Synthetic User (Likely Good)", Transaction{
			AccountAgeDays: rand.Intn(2000) + 300,
			DeviceFingerprint: "trusted_device_" + fmt.Sprint(rand.Intn(1000)),
			IPDistanceFromBillingKm: rand.Intn(50),
			IsVPNOrProxy: false,
			Velocity1Hr: rand.Intn(2) + 1,
			MerchantCategory: cat,
			AmountUSD: float64(rand.Intn(100)) + rand.Float64(),
			HistoricalAvgAmountUSD: float64(rand.Intn(150)) + 20,
		}}
	}
}

// --- Jev API Models ---
type JevRequest struct {
	Model     string                 `json:"model"`
	State     string                 `json:"state"`
	Questions map[string]interface{} `json:"questions"`
}

type JevResponse struct {
	Choices map[string]struct {
		Choice     string  `json:"choice"`
		Confidence float64 `json:"confidence"`
	} `json:"choices"`
	Nouls map[string]struct {
		Noul       bool    `json:"noul"`
		Confidence float64 `json:"confidence"`
	} `json:"nouls"`
	Scores map[string]struct {
		Score      string  `json:"score"`
		Confidence float64 `json:"confidence"`
	} `json:"scores"`
}

// --- HTML Templates ---
var indexHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Jev AI Sandbox</title>
    <script src="https://unpkg.com/htmx.org@1.9.10"></script>
    <script defer src="https://cdn.jsdelivr.net/npm/alpinejs@3.13.3/dist/cdn.min.js"></script>
    <script src="https://cdn.tailwindcss.com"></script>
	<link href="https://fonts.googleapis.com/css2?family=Inter:wght@300;400;500;600;700&family=Fira+Code:wght@400;500&display=swap" rel="stylesheet">
	<style>
		body { font-family: 'Inter', sans-serif; background-color: #050505; color: #f8fafc; }
		pre, code { font-family: 'Fira Code', monospace; }
		.htmx-indicator{display:none;}
		.htmx-request .htmx-indicator{display:inline-block;}
		.htmx-request.htmx-indicator{display:inline-block;}
	</style>
</head>
<body x-data="{ tab: 'risk' }" class="min-h-screen flex flex-col antialiased selection:bg-indigo-500/30 relative overflow-x-hidden">

	<!-- Subtle background glow -->
	<div class="absolute top-0 inset-x-0 h-[500px] bg-gradient-to-b from-indigo-900/10 via-purple-900/5 to-transparent pointer-events-none -z-10"></div>

	<!-- Header -->
	<header class="border-b border-white/5 bg-[#050505]/70 backdrop-blur-lg sticky top-0 z-40">
		<div class="max-w-5xl mx-auto px-6 h-16 flex items-center justify-between">
			<div class="flex items-center gap-3">
				<div class="w-8 h-8 rounded-lg bg-gradient-to-tr from-indigo-500 to-cyan-400 flex items-center justify-center shadow-lg shadow-indigo-500/20">
					<svg class="w-4 h-4 text-white" fill="none" viewBox="0 0 24 24" stroke="currentColor"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2.5" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
				</div>
				<span class="font-semibold text-sm tracking-wide text-slate-200">Jev <span class="text-slate-500 font-normal">Playground</span></span>
			</div>
			<div class="flex gap-1 bg-white/[0.03] p-1 rounded-lg border border-white/5">
				<button @click="tab = 'risk'" :class="tab === 'risk' ? 'bg-white/10 text-white shadow-sm ring-1 ring-white/10' : 'text-slate-400 hover:text-slate-200'" class="px-4 py-1.5 text-xs font-medium rounded-md transition-all">Risk Router</button>
				<button @click="tab = 'support'" :class="tab === 'support' ? 'bg-white/10 text-white shadow-sm ring-1 ring-white/10' : 'text-slate-400 hover:text-slate-200'" class="px-4 py-1.5 text-xs font-medium rounded-md transition-all">Support Triage</button>
				<button @click="tab = 'content'" :class="tab === 'content' ? 'bg-white/10 text-white shadow-sm ring-1 ring-white/10' : 'text-slate-400 hover:text-slate-200'" class="px-4 py-1.5 text-xs font-medium rounded-md transition-all">Moderation</button>
			</div>
		</div>
	</header>

	<main class="flex-1 max-w-5xl mx-auto w-full px-6 py-12">
		
		<!-- TAB 1: RISK ROUTER -->
		<div x-show="tab === 'risk'" x-transition.opacity.duration.300ms>
			<div class="mb-10 text-center max-w-2xl mx-auto">
				<div class="inline-flex items-center px-3 py-1 rounded-full border border-indigo-500/30 bg-indigo-500/10 text-indigo-300 text-[10px] font-bold uppercase tracking-widest mb-4">
					System One Engine
				</div>
				<h1 class="text-4xl sm:text-5xl font-extrabold mb-4 tracking-tight text-transparent bg-clip-text bg-gradient-to-r from-slate-100 to-slate-500">Payment Risk Gateway</h1>
				<p class="text-slate-400 text-sm sm:text-base leading-relaxed">Evaluate structured JSON payloads dynamically to catch Account Takeover (ATO) and Carding patterns without maintaining brittle rules engines.</p>
			</div>
			
			<div class="bg-[#0a0a0a] border border-white/10 rounded-2xl p-6 sm:p-8 shadow-2xl ring-1 ring-white/5 relative overflow-hidden">
				<div class="absolute inset-0 bg-gradient-to-br from-indigo-500/5 to-transparent pointer-events-none"></div>
				
				<form hx-post="/simulate?usecase=risk" hx-target="#risk-results" hx-indicator="#risk-spinner" class="relative z-10 mb-2">
					<fieldset>
						<legend class="text-xs font-semibold text-slate-500 mb-4 uppercase tracking-widest">Select Transaction Profile</legend>
						<div class="grid grid-cols-2 md:grid-cols-4 gap-3">
							<label class="cursor-pointer relative">
								<input type="radio" name="profile" value="random_preset" class="peer sr-only" checked>
								<div class="px-4 py-3 rounded-xl border border-white/10 bg-white/5 text-xs font-medium hover:bg-white/10 peer-checked:bg-indigo-500/20 peer-checked:border-indigo-500/50 peer-checked:text-indigo-300 transition-all text-center flex items-center justify-center h-full">🎲 Random Preset</div>
							</label>
							<label class="cursor-pointer relative">
								<input type="radio" name="profile" value="Alice" class="peer sr-only">
								<div class="px-4 py-3 rounded-xl border border-white/10 bg-white/5 text-xs font-medium hover:bg-white/10 peer-checked:bg-emerald-500/20 peer-checked:border-emerald-500/50 peer-checked:text-emerald-300 transition-all text-center flex items-center justify-center h-full">👤 Alice (Spender)</div>
							</label>
							<label class="cursor-pointer relative">
								<input type="radio" name="profile" value="Bob" class="peer sr-only">
								<div class="px-4 py-3 rounded-xl border border-white/10 bg-white/5 text-xs font-medium hover:bg-white/10 peer-checked:bg-emerald-500/20 peer-checked:border-emerald-500/50 peer-checked:text-emerald-300 transition-all text-center flex items-center justify-center h-full">👤 Bob (Big Ticket)</div>
							</label>
							<label class="cursor-pointer relative">
								<input type="radio" name="profile" value="Carol" class="peer sr-only">
								<div class="px-4 py-3 rounded-xl border border-white/10 bg-white/5 text-xs font-medium hover:bg-white/10 peer-checked:bg-emerald-500/20 peer-checked:border-emerald-500/50 peer-checked:text-emerald-300 transition-all text-center flex items-center justify-center h-full">👤 Carol (Traveler)</div>
							</label>
							<label class="cursor-pointer relative">
								<input type="radio" name="profile" value="Eve" class="peer sr-only">
								<div class="px-4 py-3 rounded-xl border border-white/10 bg-white/5 text-xs font-medium hover:bg-white/10 peer-checked:bg-rose-500/20 peer-checked:border-rose-500/50 peer-checked:text-rose-300 transition-all text-center flex items-center justify-center h-full">⚠️ Eve (ATO Fraud)</div>
							</label>
							<label class="cursor-pointer relative">
								<input type="radio" name="profile" value="Mallory" class="peer sr-only">
								<div class="px-4 py-3 rounded-xl border border-white/10 bg-white/5 text-xs font-medium hover:bg-white/10 peer-checked:bg-rose-500/20 peer-checked:border-rose-500/50 peer-checked:text-rose-300 transition-all text-center flex items-center justify-center h-full">⚠️ Mallory (Carding)</div>
							</label>
							<label class="cursor-pointer relative">
								<input type="radio" name="profile" value="Trent" class="peer sr-only">
								<div class="px-4 py-3 rounded-xl border border-white/10 bg-white/5 text-xs font-medium hover:bg-white/10 peer-checked:bg-rose-500/20 peer-checked:border-rose-500/50 peer-checked:text-rose-300 transition-all text-center flex items-center justify-center h-full">⚠️ Trent (Testing)</div>
							</label>
							<label class="cursor-pointer relative">
								<input type="radio" name="profile" value="synthetic" class="peer sr-only">
								<div class="px-4 py-3 rounded-xl border border-white/10 bg-white/5 text-xs font-medium hover:bg-white/10 peer-checked:bg-purple-500/20 peer-checked:border-purple-500/50 peer-checked:text-purple-300 transition-all text-center flex items-center justify-center h-full">🧪 Fully Synthetic</div>
							</label>
						</div>
					</fieldset>
					<div class="mt-8 flex items-center justify-center sm:justify-end">
						<button type="submit" class="w-full sm:w-auto bg-white text-black hover:bg-slate-200 font-semibold py-2.5 px-8 rounded-lg shadow-lg shadow-white/10 transition-all flex items-center justify-center h-11 text-sm">
							Evaluate Transaction
							<svg id="risk-spinner" class="htmx-indicator ml-3 animate-spin h-4 w-4 text-black" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
						</button>
					</div>
				</form>
				<div id="risk-results" class="relative z-10"></div>
			</div>
		</div>

		<!-- TAB 2: SUPPORT TRIAGE -->
		<div x-show="tab === 'support'" style="display: none;" x-transition.opacity.duration.300ms>
			<div class="mb-10 text-center max-w-2xl mx-auto">
				<div class="inline-flex items-center px-3 py-1 rounded-full border border-cyan-500/30 bg-cyan-500/10 text-cyan-300 text-[10px] font-bold uppercase tracking-widest mb-4">
					Natural Language Processing
				</div>
				<h1 class="text-4xl sm:text-5xl font-extrabold mb-4 tracking-tight text-transparent bg-clip-text bg-gradient-to-r from-slate-100 to-slate-500">Support Triage</h1>
				<p class="text-slate-400 text-sm sm:text-base leading-relaxed">Evaluate raw, unstructured text to determine urgency, map to internal departments, and gauge customer sentiment in milliseconds.</p>
			</div>
			
			<div class="bg-[#0a0a0a] border border-white/10 rounded-2xl p-6 sm:p-8 shadow-2xl ring-1 ring-white/5 relative overflow-hidden">
				<div class="absolute inset-0 bg-gradient-to-br from-cyan-500/5 to-transparent pointer-events-none"></div>
				<form hx-post="/simulate?usecase=support" hx-target="#support-results" hx-indicator="#support-spinner" class="relative z-10 mb-2">
					<div class="mb-6">
						<label class="block text-xs font-semibold text-slate-500 mb-3 uppercase tracking-widest">Customer Message (State)</label>
						<textarea name="state" rows="4" class="w-full bg-white/5 border border-white/10 rounded-xl p-4 text-sm text-slate-200 focus:outline-none focus:border-cyan-500/50 focus:ring-1 focus:ring-cyan-500/50 transition-all resize-none shadow-inner">I've been trying to reset my password for 3 days and your system keeps throwing a 500 error. This is completely unacceptable, my team cannot access our dashboard!</textarea>
					</div>
					<div class="flex justify-center sm:justify-end">
						<button type="submit" class="w-full sm:w-auto bg-white text-black hover:bg-slate-200 font-semibold py-2.5 px-8 rounded-lg shadow-lg shadow-white/10 transition-all flex items-center justify-center h-11 text-sm">
							Evaluate Message
							<svg id="support-spinner" class="htmx-indicator ml-3 animate-spin h-4 w-4 text-black" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
						</button>
					</div>
				</form>
				<div id="support-results" class="relative z-10"></div>
			</div>
		</div>

		<!-- TAB 3: CONTENT MODERATION -->
		<div x-show="tab === 'content'" style="display: none;" x-transition.opacity.duration.300ms>
			<div class="mb-10 text-center max-w-2xl mx-auto">
				<div class="inline-flex items-center px-3 py-1 rounded-full border border-rose-500/30 bg-rose-500/10 text-rose-300 text-[10px] font-bold uppercase tracking-widest mb-4">
					Content Safety
				</div>
				<h1 class="text-4xl sm:text-5xl font-extrabold mb-4 tracking-tight text-transparent bg-clip-text bg-gradient-to-r from-slate-100 to-slate-500">Moderation Engine</h1>
				<p class="text-slate-400 text-sm sm:text-base leading-relaxed">Real-time classification of user-generated content to flag toxicity, spam, and severe policy violations before they reach the database.</p>
			</div>
			
			<div class="bg-[#0a0a0a] border border-white/10 rounded-2xl p-6 sm:p-8 shadow-2xl ring-1 ring-white/5 relative overflow-hidden">
				<div class="absolute inset-0 bg-gradient-to-br from-rose-500/5 to-transparent pointer-events-none"></div>
				<form hx-post="/simulate?usecase=moderation" hx-target="#mod-results" hx-indicator="#mod-spinner" class="relative z-10 mb-2">
					<div class="mb-6">
						<label class="block text-xs font-semibold text-slate-500 mb-3 uppercase tracking-widest">User Comment (State)</label>
						<textarea name="state" rows="3" class="w-full bg-white/5 border border-white/10 rounded-xl p-4 text-sm text-slate-200 focus:outline-none focus:border-rose-500/50 focus:ring-1 focus:ring-rose-500/50 transition-all resize-none shadow-inner">Anyone who uses this framework is literally an idiot and shouldn't be allowed to touch a keyboard.</textarea>
					</div>
					<div class="flex justify-center sm:justify-end">
						<button type="submit" class="w-full sm:w-auto bg-white text-black hover:bg-slate-200 font-semibold py-2.5 px-8 rounded-lg shadow-lg shadow-white/10 transition-all flex items-center justify-center h-11 text-sm">
							Evaluate Content
							<svg id="mod-spinner" class="htmx-indicator ml-3 animate-spin h-4 w-4 text-black" xmlns="http://www.w3.org/2000/svg" fill="none" viewBox="0 0 24 24"><circle class="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" stroke-width="4"></circle><path class="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z"></path></svg>
						</button>
					</div>
				</form>
				<div id="mod-results" class="relative z-10"></div>
			</div>
		</div>

	</main>
</body>
</html>`

// Standardized Result Template for all use cases
var resultTpl = template.Must(template.New("result").Parse(`
{{if .Error}}
<div class="p-4 bg-rose-500/10 border border-rose-500/50 rounded-lg text-rose-400 text-sm whitespace-pre-wrap mt-6">
	{{.Error}}
</div>
{{else}}
<div class="grid grid-cols-1 lg:grid-cols-2 gap-6 mt-8 animate-fade-in border-t border-slate-800 pt-8">
	<!-- Left: Inputs -->
	<div class="space-y-4">
		<div class="flex items-center gap-2 mb-2">
			<div class="h-2 w-2 rounded-full bg-slate-500"></div>
			<h3 class="text-sm font-semibold uppercase tracking-wider text-slate-400">The Input (State)</h3>
		</div>
		<div class="bg-slate-950 rounded-lg border border-slate-800 p-4 shadow-inner">
			<pre class="text-[11px] leading-relaxed text-slate-300 whitespace-pre-wrap overflow-x-auto">{{.StateStr}}</pre>
		</div>

		<div class="flex items-center gap-2 mt-6 mb-2">
			<div class="h-2 w-2 rounded-full bg-indigo-500"></div>
			<h3 class="text-sm font-semibold uppercase tracking-wider text-slate-400">The Rules (Questions)</h3>
		</div>
		<div class="bg-slate-950 rounded-lg border border-slate-800 p-4 shadow-inner">
			<pre class="text-[11px] leading-relaxed text-indigo-300 whitespace-pre-wrap overflow-x-auto">{{.QuestionsStr}}</pre>
		</div>
	</div>

	<!-- Right: Results -->
	<div class="space-y-4">
		<div class="flex items-center gap-2 mb-2">
			<div class="h-2 w-2 rounded-full bg-emerald-500"></div>
			<h3 class="text-sm font-semibold uppercase tracking-wider text-slate-400">Jev Engine Decision</h3>
		</div>
		<div class="bg-slate-950 rounded-lg border border-slate-800 p-4 shadow-inner space-y-3">
			{{range $key, $val := .Decisions}}
			<div class="flex justify-between items-center border-b border-slate-800/60 pb-3 last:border-0 last:pb-0">
				<div>
					<div class="text-xs text-slate-500 font-mono">{{$key}}</div>
					<div class="text-sm font-medium text-slate-200 mt-1">{{$val.Value}}</div>
				</div>
				<div class="text-right">
					<div class="text-[10px] text-slate-500 uppercase">Confidence</div>
					<div class="text-xs font-mono {{if gt $val.Confidence 0.8}}text-emerald-400{{else}}text-amber-400{{end}}">
						{{printf "%.2f" $val.Confidence}}
					</div>
				</div>
			</div>
			{{end}}
		</div>

		<!-- Token Cost Footer -->
		<div class="mt-6 flex items-center justify-between px-4 py-3 bg-slate-900 border border-slate-800 rounded-lg text-xs font-mono text-slate-500">
			<div class="flex items-center gap-2">
				<svg class="w-4 h-4 text-slate-600" fill="none" stroke="currentColor" viewBox="0 0 24 24"><path stroke-linecap="round" stroke-linejoin="round" stroke-width="2" d="M13 10V3L4 14h7v7l9-11h-7z"></path></svg>
				Latency: {{.Latency}}ms
			</div>
			<div class="flex items-center gap-4">
				<span>Inputs: ~{{.EstTokens}} tokens</span>
				<span class="text-indigo-400 font-semibold bg-indigo-500/10 px-2 py-1 rounded">Cost: ${{printf "%.6f" .EstCost}}</span>
			</div>
		</div>
	</div>
</div>
{{end}}
`))

func init() {
	rand.Seed(time.Now().UnixNano())
}

func main() {
	godotenv.Load() // Loads .env if it exists, ignores otherwise

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html")
		w.Write([]byte(indexHTML))
	})

	http.HandleFunc("/simulate", handleSimulate)

	fmt.Println("Server running on http://localhost:8080")
	http.ListenAndServe(":8080", nil)
}

type Decision struct {
	Value      string
	Confidence float64
}

var (
	rateLimiterMu sync.Mutex
	rateLimiter   = make(map[string]time.Time)
)

func handleSimulate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// 1. Security: Max Body Size (10 KB max) to prevent large payload attacks / token burning
	r.Body = http.MaxBytesReader(w, r.Body, 10240)
	if err := r.ParseForm(); err != nil {
		renderError(w, "Payload too large or malformed.")
		return
	}

	// 2. Security: IP-based Rate Limiting (1 request per 3 seconds per IP)
	ip := strings.Split(r.RemoteAddr, ":")[0]
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		ip = strings.Split(forwarded, ",")[0]
	}

	rateLimiterMu.Lock()
	lastReq, exists := rateLimiter[ip]
	if exists && time.Since(lastReq) < 3*time.Second {
		rateLimiterMu.Unlock()
		renderError(w, "Security Check: Rate limit exceeded. Please wait 3 seconds before trying again to prevent API abuse.")
		return
	}
	rateLimiter[ip] = time.Now()
	rateLimiterMu.Unlock()

	apiKey := os.Getenv("TYPESAFE_API_KEY")
	if apiKey == "" {
		renderError(w, "TYPESAFE_API_KEY environment variable is missing!\nExport it in your terminal: export TYPESAFE_API_KEY=\"...\"")
		return
	}

	usecase := r.URL.Query().Get("usecase")
	var stateStr string
	var questions map[string]interface{}

	if usecase == "risk" {
		profileName := r.FormValue("profile")
		var prof Profile
		
		if profileName == "synthetic" {
			prof = generateSynthetic()
		} else {
			profiles := getProfiles()
			if profileName == "random_preset" {
				prof = profiles[rand.Intn(len(profiles))]
			} else {
				for _, p := range profiles {
					if strings.HasPrefix(p.Name, profileName) {
						prof = p
						break
					}
				}
				if prof.Name == "" {
					prof = profiles[0]
				}
			}
		}

		// Jitter
		prof.Tx.AmountUSD += (rand.Float64()*10 - 5)
		if prof.Tx.AmountUSD < 0.50 { prof.Tx.AmountUSD = 0.50 }
		
		pb, _ := json.MarshalIndent(prof.Tx, "", "  ")
		stateStr = string(pb)

		questions = map[string]interface{}{
			"routing_action": map[string]interface{}{
				"type": "choice",
				"instructions": "Decide the routing action.",
				"criteria": map[string]string{
					"APPROVE": "Low risk, normal behavior.",
					"STEP_UP_3DS": "Medium risk, out of pattern.",
					"DECLINE": "Obvious fraud.",
				},
			},
			"risk_severity": map[string]interface{}{
				"type": "score", "instructions": "Rate fraud risk.",
				"criteria": []string{"1 (Safe)", "2 (Low)", "3 (Med)", "4 (High)", "5 (Critical)"},
			},
			"is_ato_risk": map[string]interface{}{
				"type": "noul", "instructions": "Is this an account takeover?",
			},
		}

	} else if usecase == "support" {
		// 3. Security: Input Truncation to save tokens
		stateStr = r.FormValue("state")
		if len(stateStr) > 1000 {
			stateStr = stateStr[:1000]
		}
		questions = map[string]interface{}{
			"department": map[string]interface{}{
				"type": "choice", "instructions": "Which department?",
				"criteria": map[string]string{
					"billing": "Payments, invoices",
					"technical": "Bugs, outages",
					"sales": "Upgrades, pricing",
				},
			},
			"urgency": map[string]interface{}{
				"type": "score", "instructions": "How urgent?",
				"criteria": []string{"Low", "Medium", "High", "Critical (System down)"},
			},
			"is_frustrated": map[string]interface{}{
				"type": "noul", "instructions": "Is the user angry or frustrated?",
			},
		}
	} else if usecase == "moderation" {
		// 3. Security: Input Truncation to save tokens
		stateStr = r.FormValue("state")
		if len(stateStr) > 1000 {
			stateStr = stateStr[:1000]
		}
		questions = map[string]interface{}{
			"violation_type": map[string]interface{}{
				"type": "choice", "instructions": "What violation occurred?",
				"criteria": map[string]string{
					"none": "No violation",
					"harassment": "Attacking an individual",
					"hate_speech": "Slurs/discrimination",
				},
			},
			"is_toxic": map[string]interface{}{
				"type": "noul", "instructions": "Is this comment highly toxic or abusive?",
			},
		}
	}

	reqBody := JevRequest{
		Model: "jev-latest",
		State: stateStr,
		Questions: questions,
	}
	reqJSON, _ := json.Marshal(reqBody)
	
	// Token & Cost Estimation
	estTokens := (len(stateStr) + len(reqJSON)) / 4
	estCost := float64(estTokens) * (0.042 / 1000000.0)

	start := time.Now()
	httpReq, _ := http.NewRequest("POST", "https://api.typesafe.ai/v1/systemone", bytes.NewBuffer(reqJSON))
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(httpReq)
	latency := time.Since(start).Milliseconds()

	if err != nil {
		renderError(w, fmt.Sprintf("Failed to call Jev API: %v", err))
		return
	}
	defer resp.Body.Close()

	respBytes, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		renderError(w, fmt.Sprintf("API Error (Status %d): %s", resp.StatusCode, string(respBytes)))
		return
	}

	var jevResp JevResponse
	if err := json.Unmarshal(respBytes, &jevResp); err != nil {
		renderError(w, fmt.Sprintf("Failed to parse API response: %v\nRaw: %s", err, string(respBytes)))
		return
	}

	decisions := make(map[string]Decision)
	for k, v := range jevResp.Choices { decisions[k] = Decision{v.Choice, v.Confidence} }
	for k, v := range jevResp.Scores { decisions[k] = Decision{v.Score, v.Confidence} }
	for k, v := range jevResp.Nouls { decisions[k] = Decision{fmt.Sprintf("%t", v.Noul), v.Confidence} }

	qBytes, _ := json.MarshalIndent(questions, "", "  ")

	data := struct {
		Error        string
		StateStr     string
		QuestionsStr string
		Decisions    map[string]Decision
		EstTokens    int
		EstCost      float64
		Latency      int64
	}{
		StateStr:     stateStr,
		QuestionsStr: string(qBytes),
		Decisions:    decisions,
		EstTokens:    estTokens,
		EstCost:      estCost,
		Latency:      latency,
	}

	resultTpl.Execute(w, data)
}

func renderError(w http.ResponseWriter, errMsg string) {
	resultTpl.Execute(w, map[string]interface{}{"Error": errMsg})
}
