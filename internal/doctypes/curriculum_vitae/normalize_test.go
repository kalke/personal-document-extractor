package curriculum_vitae_test

import (
	"encoding/json"
	"testing"

	"github.com/kalke/personal-document-extractor/internal/doctypes/curriculum_vitae"
)

func TestNormalize(t *testing.T) {
	end := "12/2022"
	r := &curriculum_vitae.Result{
		FullName: "  Jane Doe  ",
		Email:    " Jane.Doe@Example.COM ",
		Phone:    "+55 (11) 98888-7777",
		Skills: curriculum_vitae.Skills{
			{Category: "backend", Items: []string{" Go ", "go"}},
			{Category: "frontend", Items: []string{"React", ""}},
		},
		Languages: []curriculum_vitae.Language{
			{Name: " English ", Level: " Fluent "},
			{Name: "", Level: "x"},
		},
		Experience: []curriculum_vitae.Experience{
			{
				Company:    " Acme ",
				Title:      " Engineer ",
				StartDate:  "01/2020",
				EndDate:    &end,
				Current:    false,
				Highlights: []string{" Built APIs ", ""},
			},
			{Company: "", Title: ""},
		},
		Education: []curriculum_vitae.Education{
			{Institution: " USP ", Degree: " BS ", StartDate: "março 2015", EndDate: "dezembro 2019"},
		},
		Certifications: []curriculum_vitae.Certification{
			{Name: " AWS SAA ", Issuer: " Amazon ", Date: "2023-06"},
			{Name: ""},
		},
	}
	curriculum_vitae.DocType{}.Normalize(r)

	if r.FullName != "Jane Doe" {
		t.Fatalf("full_name: %q", r.FullName)
	}
	if r.Email != "jane.doe@example.com" {
		t.Fatalf("email: %q", r.Email)
	}
	if r.Phone != "+5511988887777" {
		t.Fatalf("phone: %q", r.Phone)
	}
	if len(r.Skills) != 2 {
		t.Fatalf("skills groups: %#v", r.Skills)
	}
	if r.Skills[0].Category != "frontend" || len(r.Skills[0].Items) != 1 || r.Skills[0].Items[0] != "React" {
		t.Fatalf("frontend skills: %#v", r.Skills[0])
	}
	if r.Skills[1].Category != "backend" || len(r.Skills[1].Items) != 1 || r.Skills[1].Items[0] != "Go" {
		t.Fatalf("backend skills: %#v", r.Skills[1])
	}
	if len(r.Languages) != 1 || r.Languages[0].Name != "English" || r.Languages[0].Level != "Fluent" {
		t.Fatalf("languages: %#v", r.Languages)
	}
	if len(r.Experience) != 1 {
		t.Fatalf("experience len: %d", len(r.Experience))
	}
	if r.Experience[0].StartDate != "2020-01" {
		t.Fatalf("start_date: %q", r.Experience[0].StartDate)
	}
	if r.Experience[0].EndDate == nil || *r.Experience[0].EndDate != "2022-12" {
		t.Fatalf("end_date: %v", r.Experience[0].EndDate)
	}
	if len(r.Experience[0].Highlights) != 1 {
		t.Fatalf("highlights: %#v", r.Experience[0].Highlights)
	}
	if r.Education[0].StartDate != "2015-03" || r.Education[0].EndDate != "2019-12" {
		t.Fatalf("education dates: %#v", r.Education[0])
	}
	if len(r.Certifications) != 1 || r.Certifications[0].Date != "2023-06" {
		t.Fatalf("certifications: %#v", r.Certifications)
	}
}

func TestNormalizeCurrentClearsEndDate(t *testing.T) {
	end := "2024-01"
	r := &curriculum_vitae.Result{
		Experience: []curriculum_vitae.Experience{{
			Company: "X", Title: "Y", Current: true, EndDate: &end,
		}},
	}
	curriculum_vitae.DocType{}.Normalize(r)
	if r.Experience[0].EndDate != nil {
		t.Fatalf("expected nil end_date for current role, got %v", *r.Experience[0].EndDate)
	}
}

func TestNormalizeRejectsBadEmail(t *testing.T) {
	r := &curriculum_vitae.Result{Email: "not-an-email"}
	curriculum_vitae.DocType{}.Normalize(r)
	if r.Email != "" {
		t.Fatalf("expected empty email, got %q", r.Email)
	}
}

func TestSkillsJSONShapes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		raw  string
		want int
		cat  string
	}{
		{"groups", `[{"category":"Frontend","items":["React","TS"]}]`, 1, "frontend"},
		{"flat", `["Go","React"]`, 1, "other"},
		{"map", `{"backend":["Go"],"devops":["Docker"]}`, 2, "backend"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var r curriculum_vitae.Result
			if err := json.Unmarshal([]byte(`{"skills":`+tc.raw+`}`), &r); err != nil {
				t.Fatal(err)
			}
			curriculum_vitae.DocType{}.Normalize(&r)
			if len(r.Skills) != tc.want {
				t.Fatalf("got %#v", r.Skills)
			}
			if r.Skills[0].Category != tc.cat {
				t.Fatalf("first cat %q want %q", r.Skills[0].Category, tc.cat)
			}
		})
	}
}
