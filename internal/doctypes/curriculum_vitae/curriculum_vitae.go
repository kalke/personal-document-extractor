package curriculum_vitae

import (
	"encoding/json"
	"net/mail"
	"strings"
	"time"
	"unicode"

	"github.com/kalke/personal-document-extractor/internal/normalize"
)

const Name = "curriculum_vitae"

type Language struct {
	Name  string `json:"name"`
	Level string `json:"level"`
}

type Experience struct {
	Company    string   `json:"company"`
	Title      string   `json:"title"`
	Location   string   `json:"location"`
	StartDate  string   `json:"start_date"`
	EndDate    *string  `json:"end_date"`
	Current    bool     `json:"current"`
	Highlights []string `json:"highlights"`
}

type Education struct {
	Institution string `json:"institution"`
	Degree      string `json:"degree"`
	Field       string `json:"field"`
	StartDate   string `json:"start_date"`
	EndDate     string `json:"end_date"`
	Details     string `json:"details"`
}

type Certification struct {
	Name   string `json:"name"`
	Issuer string `json:"issuer"`
	Date   string `json:"date"`
}

type SkillGroup struct {
	Category string   `json:"category"`
	Items    []string `json:"items"`
}

// Skills is a categorized skill list. JSON may be:
//
//	[{"category":"frontend","items":["React"]}, ...]
//	{"frontend":["React"],"backend":["Go"]}
//	["React","Go"]  (legacy flat list → category "other")
type Skills []SkillGroup

type Result struct {
	FullName       string          `json:"full_name"`
	Headline       string          `json:"headline"`
	Email          string          `json:"email"`
	Phone          string          `json:"phone"`
	Location       string          `json:"location"`
	LinkedIn       string          `json:"linkedin"`
	GitHub         string          `json:"github"`
	Website        string          `json:"website"`
	Summary        string          `json:"summary"`
	Skills         Skills          `json:"skills"`
	Languages      []Language      `json:"languages"`
	Experience     []Experience    `json:"experience"`
	Education      []Education     `json:"education"`
	Certifications []Certification `json:"certifications"`
}

type DocType struct{}

func (DocType) Name() string { return Name }

func (DocType) SystemPrompt() string {
	return `You read a curriculum vitae / resume (CV) in Portuguese or English and return one JSON object. No markdown.

Rules:
- Extract only what is clearly written. Do not invent employers, schools, dates, skills, or contact details.
- Prefer the candidate's primary name as printed near the top.
- headline = short professional title/tagline if present (e.g. "Software Engineer"), else "".
- email / phone / location / linkedin / github / website: only when clearly labeled or obvious (mailto, linkedin.com, github.com, http(s)).
- summary = short professional summary/objective if present; do not invent one.
- skills: group discrete skill names by type. Prefer categories: frontend, backend, mobile, devops, cloud, data, tools, soft, other. Deduplicate; keep concise. Do not invent skills.
- languages: spoken/written languages with level when shown (e.g. Fluent, Native, C1); else level "".
- experience: work history newest first when order is clear. current=true only if the role is marked present/atual/hoje or has no end date while clearly ongoing. highlights = bullet achievements; omit fluff.
- education: degrees/courses; details for GPA, honors, thesis only if present.
- certifications: name + issuer + date when available.
- Dates: prefer YYYY-MM for month-precision (common on CVs) or YYYY-MM-DD when a full day is shown. Accept MM/YYYY, MM/YY, Month YYYY, DD/MM/YYYY. Unknown dates "". Ongoing end_date = null when current=true.
- Unknown text fields "". Unknown lists []. Unknown nullable end_date null.
- Ignore photos, QR codes, and decorative text.`
}

func (DocType) SchemaHint() string {
	return `{
  "full_name": "string",
  "headline": "string",
  "email": "string",
  "phone": "string",
  "location": "string — city/region/country as printed",
  "linkedin": "string — URL or handle",
  "github": "string — URL or handle",
  "website": "string — URL",
  "summary": "string",
  "skills": [
    {"category": "frontend|backend|mobile|devops|cloud|data|tools|soft|other", "items": ["string"]}
  ],
  "languages": [{"name": "string", "level": "string"}],
  "experience": [{
    "company": "string",
    "title": "string",
    "location": "string",
    "start_date": "YYYY-MM or YYYY-MM-DD or \"\"",
    "end_date": "YYYY-MM or YYYY-MM-DD or null",
    "current": false,
    "highlights": ["string"]
  }],
  "education": [{
    "institution": "string",
    "degree": "string",
    "field": "string",
    "start_date": "YYYY-MM or YYYY-MM-DD or \"\"",
    "end_date": "YYYY-MM or YYYY-MM-DD or \"\"",
    "details": "string"
  }],
  "certifications": [{"name": "string", "issuer": "string", "date": "YYYY-MM or YYYY-MM-DD or \"\""}]
}`
}

func (DocType) EmptyResult() any { return &Result{} }

func (DocType) Normalize(result any) {
	r, ok := result.(*Result)
	if !ok || r == nil {
		return
	}
	r.FullName = collapse(r.FullName)
	r.Headline = collapse(r.Headline)
	r.Email = normalizeEmail(r.Email)
	r.Phone = normalizePhone(r.Phone)
	r.Location = collapse(r.Location)
	r.LinkedIn = collapse(r.LinkedIn)
	r.GitHub = collapse(r.GitHub)
	r.Website = collapse(r.Website)
	r.Summary = collapse(r.Summary)
	r.Skills = cleanSkills(r.Skills)
	r.Languages = cleanLanguages(r.Languages)
	r.Experience = cleanExperience(r.Experience)
	r.Education = cleanEducation(r.Education)
	r.Certifications = cleanCertifications(r.Certifications)
}

func collapse(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func (s *Skills) UnmarshalJSON(data []byte) error {
	data = bytesTrimSpace(data)
	if len(data) == 0 || string(data) == "null" {
		*s = Skills{}
		return nil
	}
	switch data[0] {
	case '[':
		var groups []SkillGroup
		if err := json.Unmarshal(data, &groups); err == nil {
			*s = groups
			return nil
		}
		var flat []string
		if err := json.Unmarshal(data, &flat); err != nil {
			return err
		}
		*s = Skills{{Category: "other", Items: flat}}
		return nil
	case '{':
		var m map[string][]string
		if err := json.Unmarshal(data, &m); err != nil {
			return err
		}
		out := make(Skills, 0, len(m))
		for cat, items := range m {
			out = append(out, SkillGroup{Category: cat, Items: items})
		}
		*s = out
		return nil
	default:
		return json.Unmarshal(data, (*[]SkillGroup)(s))
	}
}

func bytesTrimSpace(b []byte) []byte {
	return []byte(strings.TrimSpace(string(b)))
}

var skillCategoryOrder = []string{
	"frontend", "backend", "mobile", "devops", "cloud", "data", "tools", "soft", "other",
}

var skillCategoryAliases = map[string]string{
	"frontend": "frontend", "front-end": "frontend", "front end": "frontend",
	"front": "frontend", "ui": "frontend", "web": "frontend",
	"backend": "backend", "back-end": "backend", "back end": "backend",
	"back": "backend", "server": "backend", "api": "backend",
	"mobile": "mobile", "android": "mobile", "ios": "mobile",
	"devops": "devops", "sre": "devops", "infra": "devops", "infrastructure": "devops",
	"ops": "devops", "platform": "devops",
	"cloud": "cloud", "aws": "cloud", "gcp": "cloud", "azure": "cloud",
	"data": "data", "ml": "data", "ai": "data", "analytics": "data", "database": "data", "databases": "data",
	"tools": "tools", "tooling": "tools", "productivity": "tools",
	"soft": "soft", "soft skills": "soft", "softskills": "soft", "interpersonal": "soft",
	"other": "other", "misc": "other", "general": "other", "languages": "other",
	"programming languages": "other", "linguagens": "other",
}

func canonicalizeSkillCategory(s string) string {
	key := strings.ToLower(collapse(s))
	if key == "" {
		return "other"
	}
	if canon, ok := skillCategoryAliases[key]; ok {
		return canon
	}
	for _, known := range skillCategoryOrder {
		if strings.Contains(key, known) {
			return known
		}
	}
	return "other"
}

func cleanSkills(in Skills) Skills {
	merged := map[string][]string{}
	seenInCat := map[string]map[string]struct{}{}
	for _, g := range in {
		cat := canonicalizeSkillCategory(g.Category)
		if seenInCat[cat] == nil {
			seenInCat[cat] = map[string]struct{}{}
		}
		for _, item := range cleanStrings(g.Items) {
			key := strings.ToLower(item)
			if _, ok := seenInCat[cat][key]; ok {
				continue
			}
			seenInCat[cat][key] = struct{}{}
			merged[cat] = append(merged[cat], item)
		}
	}
	out := make(Skills, 0, len(skillCategoryOrder))
	for _, cat := range skillCategoryOrder {
		items := merged[cat]
		if len(items) == 0 {
			continue
		}
		out = append(out, SkillGroup{Category: cat, Items: items})
	}
	return out
}

func normalizeEmail(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	addr, err := mail.ParseAddress(s)
	if err != nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(addr.Address))
}

func normalizePhone(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	var b strings.Builder
	for i, r := range s {
		if unicode.IsDigit(r) {
			b.WriteRune(r)
			continue
		}
		if r == '+' && i == 0 {
			b.WriteRune(r)
			continue
		}
	}
	d := b.String()
	digits := normalize.DigitsOnly(d)
	if len(digits) < 8 {
		return ""
	}
	return d
}

func cleanStrings(in []string) []string {
	if len(in) == 0 {
		return []string{}
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = collapse(s)
		if s == "" {
			continue
		}
		key := strings.ToLower(s)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, s)
	}
	return out
}

func cleanLanguages(in []Language) []Language {
	if len(in) == 0 {
		return []Language{}
	}
	out := make([]Language, 0, len(in))
	for _, lang := range in {
		name := collapse(lang.Name)
		if name == "" {
			continue
		}
		out = append(out, Language{Name: name, Level: collapse(lang.Level)})
	}
	return out
}

func cleanExperience(in []Experience) []Experience {
	if len(in) == 0 {
		return []Experience{}
	}
	out := make([]Experience, 0, len(in))
	for _, exp := range in {
		company := collapse(exp.Company)
		title := collapse(exp.Title)
		if company == "" && title == "" {
			continue
		}
		start := normalizeCVDate(exp.StartDate)
		var end *string
		if exp.Current {
			end = nil
		} else if exp.EndDate != nil {
			if n := normalizeCVDate(*exp.EndDate); n != "" {
				end = &n
			} else {
				end = nil
			}
		}
		out = append(out, Experience{
			Company:    company,
			Title:      title,
			Location:   collapse(exp.Location),
			StartDate:  start,
			EndDate:    end,
			Current:    exp.Current,
			Highlights: cleanStrings(exp.Highlights),
		})
	}
	return out
}

func cleanEducation(in []Education) []Education {
	if len(in) == 0 {
		return []Education{}
	}
	out := make([]Education, 0, len(in))
	for _, ed := range in {
		inst := collapse(ed.Institution)
		degree := collapse(ed.Degree)
		if inst == "" && degree == "" {
			continue
		}
		out = append(out, Education{
			Institution: inst,
			Degree:      degree,
			Field:       collapse(ed.Field),
			StartDate:   normalizeCVDate(ed.StartDate),
			EndDate:     normalizeCVDate(ed.EndDate),
			Details:     collapse(ed.Details),
		})
	}
	return out
}

func cleanCertifications(in []Certification) []Certification {
	if len(in) == 0 {
		return []Certification{}
	}
	out := make([]Certification, 0, len(in))
	for _, c := range in {
		name := collapse(c.Name)
		if name == "" {
			continue
		}
		out = append(out, Certification{
			Name:   name,
			Issuer: collapse(c.Issuer),
			Date:   normalizeCVDate(c.Date),
		})
	}
	return out
}

// normalizeCVDate returns YYYY-MM, YYYY-MM-DD, or "".
func normalizeCVDate(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if iso := normalize.DateToISO(s); iso != nil {
		return *iso
	}
	s = strings.ReplaceAll(s, ".", "/")
	s = strings.ReplaceAll(s, "-", "/")
	s = strings.Join(strings.Fields(s), " ")

	monthLayouts := []string{
		"2006/01",
		"2006/1",
		"01/2006",
		"1/2006",
		"01/06",
		"1/06",
		"January 2006",
		"Jan 2006",
		"January/2006",
		"Jan/2006",
	}
	for _, layout := range monthLayouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01")
		}
	}
	// Portuguese month names (common on BR CVs).
	pt := map[string]string{
		"janeiro": "January", "fevereiro": "February", "março": "March", "marco": "March",
		"abril": "April", "maio": "May", "junho": "June", "julho": "July",
		"agosto": "August", "setembro": "September", "outubro": "October",
		"novembro": "November", "dezembro": "December",
		"jan": "Jan", "fev": "Feb", "mar": "Mar", "abr": "Apr", "mai": "May",
		"jun": "Jun", "jul": "Jul", "ago": "Aug", "set": "Sep", "out": "Oct",
		"nov": "Nov", "dez": "Dec",
	}
	lower := strings.ToLower(s)
	for ptName, enName := range pt {
		if !strings.HasPrefix(lower, ptName) {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(lower, ptName))
		rest = strings.TrimLeft(rest, " /.-")
		try := enName + " " + rest
		for _, layout := range []string{"January 2006", "Jan 2006"} {
			if t, err := time.Parse(layout, try); err == nil {
				return t.Format("2006-01")
			}
		}
	}
	return ""
}
