package smtp

import (
	"fmt"
	"net/mail"
	"strings"
)

type normalizedRecipients struct {
	ToHeader    string
	CCHeader    string
	ToEnvelope  []string
	CCEnvelope  []string
	BCCEnvelope []string
	AllEnvelope []string
}

func normalizeMailbox(field, raw string) (header string, envelope string, err error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", "", fmt.Errorf("%s address is required", field)
	}
	addr, err := mail.ParseAddress(raw)
	if err != nil {
		return "", "", fmt.Errorf("%s address %q is invalid: %w", field, raw, err)
	}
	return formatHeaderAddress(addr), addr.Address, nil
}

func normalizeRecipientFields(to, cc, bcc string) (normalizedRecipients, error) {
	toHeader, toEnvelope, err := normalizeAddressList("To", to, true)
	if err != nil {
		return normalizedRecipients{}, err
	}
	ccHeader, ccEnvelope, err := normalizeAddressList("Cc", cc, false)
	if err != nil {
		return normalizedRecipients{}, err
	}
	_, bccEnvelope, err := normalizeAddressList("Bcc", bcc, false)
	if err != nil {
		return normalizedRecipients{}, err
	}

	all := make([]string, 0, len(toEnvelope)+len(ccEnvelope)+len(bccEnvelope))
	all = append(all, toEnvelope...)
	all = append(all, ccEnvelope...)
	all = append(all, bccEnvelope...)

	return normalizedRecipients{
		ToHeader:    toHeader,
		CCHeader:    ccHeader,
		ToEnvelope:  toEnvelope,
		CCEnvelope:  ccEnvelope,
		BCCEnvelope: bccEnvelope,
		AllEnvelope: all,
	}, nil
}

func normalizeAddressList(field, raw string, required bool) (header string, envelope []string, err error) {
	raw = trimTrailingAddressSeparators(raw)
	if raw == "" {
		if required {
			return "", nil, fmt.Errorf("%s address is required", field)
		}
		return "", nil, nil
	}

	addrs, err := mail.ParseAddressList(raw)
	if err != nil {
		return "", nil, fmt.Errorf("%s recipient %q is invalid: %w", field, raw, err)
	}
	if required && len(addrs) == 0 {
		return "", nil, fmt.Errorf("%s address is required", field)
	}

	headerParts := make([]string, 0, len(addrs))
	envelope = make([]string, 0, len(addrs))
	for _, addr := range addrs {
		headerParts = append(headerParts, formatHeaderAddress(addr))
		envelope = append(envelope, addr.Address)
	}
	return strings.Join(headerParts, ", "), envelope, nil
}

func trimTrailingAddressSeparators(raw string) string {
	raw = strings.TrimSpace(raw)
	for strings.HasSuffix(raw, ",") {
		raw = strings.TrimSpace(strings.TrimSuffix(raw, ","))
	}
	return raw
}

func formatHeaderAddress(addr *mail.Address) string {
	if addr == nil {
		return ""
	}
	if addr.Name == "" {
		return addr.Address
	}
	if isSimpleDisplayName(addr.Name) {
		return fmt.Sprintf("%s <%s>", addr.Name, addr.Address)
	}
	return addr.String()
}

func isSimpleDisplayName(name string) bool {
	if strings.TrimSpace(name) != name {
		return false
	}
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z':
		case r >= 'A' && r <= 'Z':
		case r >= '0' && r <= '9':
		case r == ' ' || r == '.' || r == '_' || r == '-':
		default:
			return false
		}
	}
	return name != ""
}
