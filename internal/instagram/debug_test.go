package instagram

import "testing"

func TestExtractDebugMediaCandidatesFindsCroppedAndUncroppedURLs(t *testing.T) {
	report := DebugReport{
		Fetches: []DebugFetch{{
			Name: "original_page",
			ExtractedJSON: []DebugJSONBlock{{
				Key:   "items",
				Index: 1,
				Raw: `[
					{
						"image_versions2": {
							"candidates": [
								{
									"url": "https://scontent.cdninstagram.com/v/t51.82787-15/624883761_17999919473715120_5399055720230461434_n.jpg?stp=c288.0.864.864a_dst-jpg_e35_s640x640_tt6",
									"width": 864,
									"height": 864
								},
								{
									"url": "https://scontent.cdninstagram.com/v/t51.82787-15/624883761_17999919473715120_5399055720230461434_n.jpg?stp=dst-jpg_e35_s1080x1080_tt6",
									"width": 1080,
									"height": 1080
								}
							]
						}
					}
				]`,
			}},
		}},
	}

	candidates := ExtractDebugMediaCandidates(report)
	if len(candidates) != 2 {
		t.Fatalf("candidate count = %d, want 2: %#v", len(candidates), candidates)
	}
	if !candidates[0].Cropped {
		t.Fatalf("first candidate should be cropped: %#v", candidates[0])
	}
	if candidates[1].Cropped {
		t.Fatalf("second candidate should be uncropped: %#v", candidates[1])
	}
	if candidates[1].Width != 1080 || candidates[1].Height != 1080 {
		t.Fatalf("second candidate size = %dx%d", candidates[1].Width, candidates[1].Height)
	}
	if candidates[1].Source != "original_page items #1" {
		t.Fatalf("second candidate source = %q", candidates[1].Source)
	}
}

func TestExtractDebugMediaCandidatesScansRawBodyURLs(t *testing.T) {
	report := DebugReport{
		Fetches: []DebugFetch{{
			Name: "embed_page",
			Body: `{"display_url":"https:\/\/scontent.cdninstagram.com\/v\/t51.82787-15\/full.jpg?stp=dst-jpg_e35_s1080x1080_tt6\u0026_nc_cat=111"}`,
		}},
	}

	candidates := ExtractDebugMediaCandidates(report)
	if len(candidates) != 1 {
		t.Fatalf("candidate count = %d, want 1: %#v", len(candidates), candidates)
	}
	if candidates[0].URL != "https://scontent.cdninstagram.com/v/t51.82787-15/full.jpg?stp=dst-jpg_e35_s1080x1080_tt6&_nc_cat=111" {
		t.Fatalf("candidate URL = %q", candidates[0].URL)
	}
	if candidates[0].Source != "embed_page body" {
		t.Fatalf("candidate source = %q", candidates[0].Source)
	}
}
