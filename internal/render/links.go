package render

import (
	"net/url"
	"regexp"
	"strings"
)

// URLRe matches http/https URLs.
var URLRe = regexp.MustCompile(`https?://[^\s<>\[\](){}"'` + "`" + `]+`)

// LinkifyWrappedLines applies LinkifyURLs to each pre-wrapped line individually.
// This ensures OSC 8 escape sequences never span a line break, which would leave
// the terminal in a broken hyperlink state and corrupt adjacent panel rendering.
func LinkifyWrappedLines(lines []string) []string {
	out := make([]string, len(lines))
	for i, line := range lines {
		out[i] = LinkifyURLs(line)
	}
	return out
}

// LinkifyURLs replaces raw URLs with OSC 8 terminal hyperlinks.
// The visible text is a shortened version (domain + truncated path);
// the full URL is embedded in the escape sequence so terminals can open it on click.
func LinkifyURLs(text string) string {
	return URLRe.ReplaceAllStringFunc(text, func(raw string) string {
		trimmed := strings.TrimRight(raw, ".,;:!?)")
		label := ShortenURL(trimmed)
		// OSC 8: \033]8;;URL\033\\ LABEL \033]8;;\033\\
		return "\033]8;;" + trimmed + "\033\\" + label + "\033]8;;\033\\"
	})
}

// ShortenURL produces a human-readable label like "example.com/path…"
func ShortenURL(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		if len(raw) > 40 {
			return raw[:37] + "..."
		}
		return raw
	}
	host := parsed.Hostname()
	path := parsed.Path
	if q := parsed.RawQuery; q != "" {
		path += "?" + q
	}
	if path == "" || path == "/" {
		return host
	}
	full := host + path
	if len(full) > 50 {
		return full[:47] + "..."
	}
	return full
}

// --- Tracker / link sanitization ---

// knownTrackerParams lists URL query parameters commonly used for email
// tracking. StripTrackers removes these so the underlying destination is
// visible without click-tracking noise.
var knownTrackerParams = []string{
	// UTM campaign tracking
	"utm_source", "utm_medium", "utm_campaign", "utm_term", "utm_content", "utm_id",
	// General click/email trackers
	"mc_cid", "mc_eid",           // Mailchimp
	"_hsenc", "_hsmi", "hsa_cam", // HubSpot
	"fbclid",                     // Facebook
	"gclid", "gclsrc",           // Google Ads
	"msclkid",                    // Microsoft Ads
	"dclid",                      // DoubleClick
	"twclid",                     // Twitter
	"igshid",                     // Instagram
	"s_kwcid",                    // Adobe Analytics
	"trk", "trkCampaign", "trkInfo", // LinkedIn
	"si",                  // Spotify
	"ref", "ref_",         // Generic referrer tags
	"oly_anon_id", "oly_enc_id", // Onlytica
	"vero_id",             // Vero
	"_bta_tid", "_bta_c",  // Bronto
	"spm",                 // Taobao/Alibaba
	"wickedid",            // Wicked Reports
	"dm_i",                // dotdigital
}

// trackerParamSet is a fast lookup built from knownTrackerParams.
var trackerParamSet map[string]bool

func init() {
	trackerParamSet = make(map[string]bool, len(knownTrackerParams))
	for _, p := range knownTrackerParams {
		trackerParamSet[strings.ToLower(p)] = true
	}
}

// StripTrackers removes known tracking query parameters from a URL string.
// If all query parameters are trackers the query string is removed entirely.
// The URL is otherwise unchanged (scheme, host, path, fragment preserved).
func StripTrackers(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	q := parsed.Query()
	if len(q) == 0 {
		return rawURL
	}
	changed := false
	for key := range q {
		if trackerParamSet[strings.ToLower(key)] {
			q.Del(key)
			changed = true
		}
	}
	if !changed {
		return rawURL
	}
	parsed.RawQuery = q.Encode()
	return parsed.String()
}

// StripTrackersFromText applies StripTrackers to every URL found in text.
func StripTrackersFromText(text string) string {
	return URLRe.ReplaceAllStringFunc(text, func(raw string) string {
		trimmed := strings.TrimRight(raw, ".,;:!?)")
		suffix := raw[len(trimmed):]
		return StripTrackers(trimmed) + suffix
	})
}
