package i18n

import "fmt"

var messages = map[string]map[string]string{
	"fa": {
		"start_welcome":        "به ربات فروش VPN خوش آمدید.",
		"main_menu":            "منوی اصلی:",
		"btn_buy":              "خرید سرویس",
		"btn_services":         "سرویس‌های من",
		"btn_wallet":           "کیف پول",
		"btn_topup":            "شارژ کیف پول",
		"btn_support":          "پشتیبانی",
		"btn_language":         "زبان",
		"topup_enter_amount":   "مبلغ شارژ (تومان) را وارد کنید:",
		"topup_send_receipt":   "رسید پرداخت را به صورت عکس ارسال کنید.",
		"topup_invalid_amount": "مبلغ نامعتبر است.",
		"support_text":         "پشتیبانی: %s",
		"lang_switched":        "زبان به فارسی تغییر کرد.",
		"wallet_balance":       "موجودی: %d تومان",
		"plan_list":            "پلن‌های موجود:",
		"plan_item":            "%s - %d گیگ - %d روز - %d تومان",
		"insufficient":         "موجودی کافی نیست. از دکمه شارژ کیف پول استفاده کنید.",
		"order_ok":             "سفارش ثبت شد. در حال آماده‌سازی...",
		"order_failed":         "سفارش انجام نشد و مبلغ به کیف پول بازگشت.",
		"service_ready":        "سرویس شما آماده است:\n%s",
		"topup_pending":        "درخواست شارژ ثبت شد و در انتظار تأیید است.",
		"topup_approved":       "شارژ کیف پول تأیید شد.",
		"usage_warn":           "هشدار: %d%% از حجم سرویس مصرف شده است.",
		"service_expiring":     "سرویس شما به زودی منقضی می‌شود.",
		"service_expired":      "سرویس شما منقضی شده است.",
		"force_join_required":  "برای استفاده از ربات ابتدا در کانال %s عضو شوید.",
		"pay_method_select":    "پلن %s — %d تومان\nروش پرداخت را انتخاب کنید:",
		"btn_pay_wallet":       "پرداخت از کیف پول",
		"btn_pay_card":         "کارت به کارت",
		"plan_card_instructions": "مبلغ %d تومان را واریز کنید:",
		"user_blocked":         "دسترسی شما مسدود شده است.",
	},
	"en": {
		"start_welcome":        "Welcome to the VPN sales bot.",
		"main_menu":            "Main menu:",
		"btn_buy":              "Buy service",
		"btn_services":         "My services",
		"btn_wallet":           "Wallet",
		"btn_topup":            "Top up wallet",
		"btn_support":          "Support",
		"btn_language":         "Language",
		"topup_enter_amount":   "Enter top-up amount (Toman):",
		"topup_send_receipt":   "Send a photo of your payment receipt.",
		"topup_invalid_amount": "Invalid amount.",
		"support_text":         "Support: %s",
		"lang_switched":        "Language switched to English.",
		"wallet_balance":       "Balance: %d Toman",
		"plan_list":            "Available plans:",
		"plan_item":            "%s - %d GB - %d days - %d Toman",
		"insufficient":         "Insufficient balance. Please top up your wallet.",
		"order_ok":             "Order placed. Provisioning...",
		"order_failed":         "Order failed and the amount was refunded to your wallet.",
		"service_ready":        "Your service is ready:\n%s",
		"topup_pending":        "Top-up request submitted and pending approval.",
		"topup_approved":       "Wallet top-up approved.",
		"usage_warn":           "Warning: %d%% of your service data has been used.",
		"service_expiring":     "Your service is expiring soon.",
		"service_expired":      "Your service has expired.",
		"force_join_required":  "Please join %s before using this bot.",
		"pay_method_select":    "Plan %s — %d Toman\nChoose payment method:",
		"btn_pay_wallet":       "Pay from wallet",
		"btn_pay_card":         "Card transfer",
		"plan_card_instructions": "Transfer %d Toman:",
		"user_blocked":         "Your access has been blocked.",
	},
}

// T returns a translated string for lang/key with optional fmt args.
func T(lang, key string, args ...any) string {
	if lang == "" {
		lang = "fa"
	}
	m, ok := messages[lang]
	if !ok {
		m = messages["fa"]
	}
	s, ok := m[key]
	if !ok {
		return key
	}
	if len(args) > 0 {
		return fmt.Sprintf(s, args...)
	}
	return s
}
