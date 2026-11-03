# نقشهٔ فازها — کلون Go از MaNGOS One (2.4.3 / build 8606)

مرجع رفتار: https://github.com/mangosone/server (نسخهٔ رسمی تثبیت‌شده). مرجع schema: فایل `all.sql` و دیتابیس‌های موجود کاربر در MySQL 5.6.51. جدول‌های اضافی حفظ می‌شوند؛ هیچ رفتار Warden یا World در این مرحله پیاده‌سازی نمی‌شود.

## اصول ثابت

1. روال هر فاز: خواندن C++ مرجع → توضیح رفتار → پیاده‌سازی کوچک در Go → آزمایش → ثبت تفاوت‌های باقی‌مانده در `docs/differences.md`.
2. دیتابیس‌ها مال MaNGOS هستند (`realmd`، `character1`، `mangos1`). Go هیچ schema‌ای نمی‌سازد؛ فقط داده‌های لازم را می‌خواند و عملیات مجاز حساب/session را در جدول‌های موجود می‌نویسد.
3. هر دیتابیس یک پکیج storage و DSN مجزا دارد؛ interfaceها سمت مصرف‌کننده تعریف می‌شوند.
4. داده‌های ثابت `mangos1` فقط هنگام راه‌اندازی در حافظه (immutable) بارگذاری می‌شوند، نه در مسیر پردازش پیام.
5. codec (تابع خالص بدون I/O) / handler (state machine) / domain (منطق) از هم جدا می‌مانند.
6. همهٔ فایل‌ها `gofmt` شوند؛ تابع‌های طولانی شکسته شوند.

## وضعیت فعلی

- transport، SRP، Realm List و reconnect به schema واقعی `realmd` منتقل شده‌اند؛ جدول‌های اختصاصی و دستور `account init` از کد حذف شده‌اند.
- اتصال و بررسی ساختار هر سه دیتابیس اصلی Docker روی 3306 موفق است. تست‌های نوشتنی فقط روی کانتینر جداگانهٔ 3308 انجام می‌شوند.
- یک حساب موجود با salt/verifier کامل به‌صورت خواندنی تطبیق داده شده است. چهار حساب دیگر مقدار اولیهٔ کوتاه دارند؛ مقداردهی اولیهٔ مشابه MaNGOS روی دیتابیس تست پوشش داده شده است.
- معیار تست نهایی ورود با حساب موجود از خود کلاینت هنوز نیاز به اجرای کاربر دارد. تفاوت‌های باقیمانده در `docs/differences.md` ثبت شده‌اند.

---

## فاز ۳.۵ — انتقال به دیتابیس واقعی realmd

- مطالعهٔ `AuthSocket.cpp`، `RealmList.cpp` و ثبت قواعد: ترتیب بایت `s`/`v`/`sessionkey` (hex)، ban، lock، `realmbuilds`، `allowedSecurityLevel`.
- حذف `schema.sql`، `realms.sql`، `realmcharacters.sql`، `Migrate` و دستور `account init`.
- `storage/realmd`: `account`، `account_banned`، `ip_banned`، `realmlist`، `realmcharacters`.
- تنظیمات: سه DSN (realmd / characters / world) در config.
- `compose.yaml`: بارگذاری dump به‌عنوان init و استفاده از نسخهٔ سازگار MySQL (dump روی 5.6 است؛ سازگاری driver بررسی شود).
- رفتار مطابق مرجع: ip_banned، account_banned فعال/منقضی، locked با `last_ip`، `failed_logins`، فیلتر realm با gmlevel و build، آدرس محلی/عمومی.
- ابزار مدیریتی: `account create` (نوشتن `username`، `sha_pass_hash`، `s`، `v`)، `check`، `realm list`.

معیار پایان: هم حساب ساخته‌شده با ابزار ما و هم حساب موجود در دیتابیس با کلاینت واقعی وارد شوند. فایل dump فقط schema دارد؛ دادهٔ حساب در دیتابیس موجود است. برای جلوگیری از ثبت اطلاعات حساس، تطبیق verifier حساب واقعی در حافظه و بدون ذخیرهٔ fixture انجام می‌شود.

## فاز ۳.۶ — بازسازی لایه‌بندی

- جداسازی `codec` / `handler` / `domain` در `protocol/auth`.
- انتقال interfaceها به سمت مصرف‌کننده و ادغام `cmd/account` و `cmd/realm` در `cmd/wowctl`.
- شکستن `config.Parse` و `Handler.Serve` به توابع کوچک؛ `gofmt` و `go vet` روی کل پروژه.

معیار پایان: همهٔ تست‌های قبلی سبز و بدون تغییر رفتار.

## فاز ۴ — زیرساخت پروتکل World (پیاده‌سازی شده)

- `wire`: ByteBuffer (little-endian، cstring، packed GUID).
- تولید ثابت‌های opcode از `Opcodes.h` با ابزار در `tools/`، نه دستی.
- جدول opcode با وضعیت مجاز (`STATUS_AUTHED`، `STATUS_LOGGEDIN` …).
- `AuthCrypt` برای 2.4.3: مشتق‌سازی کلید با HMAC-SHA1 و تبدیل stateful بایت‌ها با XOR و جمع/تفریق مطابق مرجع؛ RC4 و drop مربوط به این پیاده‌سازی نیست. تست با بردار مستقل.
- framing: header سروری (اندازه + opcode) و کلاینتی، بسته‌های ناقص/چسبیده، سقف اندازه، fuzz.

معیار پایان: encode/decode و رمزنگاری با بردارهای مستقل تست شوند، بدون سوکت.

خروجی: `protocol/wire`، framing و Reader/Writer نسخهٔ TBC، رمزنگاری دو جهت،
۱۰۶۰ opcode و metadata تولیدشده از مرجع. قراردادها و تست‌ها در
`docs/world-protocol.md` توضیح داده شده‌اند. listener و handlerهای World مربوط به فاز ۵ هستند.

## فاز ۵ — اتصال worldd تا صفحهٔ کاراکتر (تأیید کلاینت واقعی توسط کاربر)

- `cmd/worldd`: `SMSG_AUTH_CHALLENGE`، `CMSG_AUTH_SESSION`، تأیید digest با session key از realmd.
- فعال‌سازی رمزنگاری header در نقطهٔ درست، `SMSG_AUTH_RESPONSE`.
- Session: goroutine خواندن و نوشتن، صف پیام، ping/pong، قطع اتصال تمیز.
- پاسخ لیست خالی کاراکتر (`SMSG_CHAR_ENUM`).

معیار پایان: بعد از انتخاب realm صفحهٔ کاراکتر باز شود و اتصال پایدار بماند.

کد و تست‌ها آماده‌اند؛ کاربر اجرای موفق با کلاینت واقعی را تأیید کرد. تست‌های خودکار اجرا نشده‌اند.
راهنمای اجرا، محدودیت‌ها و وضعیت اعتبارسنجی در `docs/worldd.md` ثبت شده‌اند.

## فاز ۶ — داده‌های ثابت (پیاده‌سازی؛ اجرای startup هنوز بررسی نشده)

- loader برای DBCهای لازم (ChrRaces، ChrClasses و …).
- بارگذاری `playercreateinfo*`، `player_levelstats`، `player_classlevelstats`، `player_xp_for_level`، `item_template` (حداقل لازم) در حافظه.
- نام‌های رزرو شده (`reserved_name`) و قواعد نام.

معیار پایان: راه‌اندازی بدون خطا با آمار بارگذاری در لاگ؛ تست روی چند ترکیب نژاد/کلاس.

loader شش DBC، جدول‌های اولیهٔ mangos1 و Catalog فقط‌خواندنی اضافه شده‌اند.
خروجی extractor موجود مصرف می‌شود. طبق درخواست کاربر تست خودکار و اجرای سرور انجام نشده؛
مشاهدهٔ لاگ startup باقی است. محدودهٔ قواعد نام و تفاوت‌های validation در `docs/static-data.md` ثبت شده‌اند.

## فاز ۷ — فهرست کاراکترها (تأییدشده با کلاینت اصلی توسط کاربر)

- `storage/characters`: خواندن `characters` فقط برای همان حساب و realm.
- `SMSG_CHAR_ENUM` شامل ظاهر، موقعیت، guild، تجهیزات (`equipmentCache`/`character_inventory` و displayid از `item_template`).
- یک کاراکتر آزمایشی با داده کنترل‌شده.

معیار پایان: کاراکتر آزمایشی با نام، ظاهر و تجهیزات درست نمایش داده شود و کاراکتر حساب دیگر دیده نشود.

خواندن فهرست حساب و serialization نسخهٔ 8606 اضافه شده‌اند. دادهٔ آزمایشی در دیتابیس
واقعی ساخته نشده است. طبق درخواست کاربر هیچ تست، build یا اجرای سرور انجام نشده؛
کاربر نمایش واقعی را تأیید کرده و تست بیشتر نخواسته است. جزئیات در `docs/characters.md`.

## فاز ۸ — ساخت کاراکتر (مسیر موفق تأییدشده با کلاینت اصلی)

- `CMSG_CHAR_CREATE`: اعتبارسنجی نام، تکراری‌بودن، ترکیب نژاد/کلاس/ظاهر، محدودیت‌های حساب و realm.
- تخصیص GUID، درج در `characters`، `item_instance`، `character_inventory`، `character_spell`، `character_action`، `character_skills`، `character_reputation`، `character_homebind` — همه در یک transaction.
- به‌روزرسانی `realmcharacters.numchars`.
- شروع با یک ترکیب نژاد/کلاس؛ ترکیب‌های پشتیبانی‌نشده صریحاً رد شوند.

معیار پایان: کاراکتر ساخته شود، در لیست بماند و پس از ری‌استارت باقی باشد؛ شکست هیچ دادهٔ نیمه‌کاره نگذارد.

ساخت Human Warrior با هر دو جنسیت، اعتبارسنجی، داده‌های اولیه و تراکنش مشترک
characters/login اضافه شده است. کاربر ساخت و نمایش را با کلاینت اصلی تأیید کرد؛
زمان storage پس از درج گروهی به حدود ۶۷۷ms رسید. ماندگاری پس از restart کلاینت واقعی
هنوز جداگانه گزارش نشده است. جزئیات و محدودیت‌ها در `docs/character-creation.md`.

## فاز ۸٫۵ — بازآرایی معماری (انجام‌شده؛ بررسی خودکار موفق)

- Session با مالکیت صریح state، حلقهٔ رویداد و Router مبتنی بر حالت.
- codec خالص world، سرویس ساخت و policy روی facts داخل همان transaction.
- adapter صریح mangosone8606 برای رکورد آیتم.
- RealmdConfig/WorldConfig و راه‌اندازی سرویس‌ها در internal/app.
- تست رقابت نام، آخرین جایگاه، سقف حساب، session نامعتبر و rollback مرحلهٔ میانی/نهایی.
- go test -race با دیتابیس مجزای آزمایشی، go vet و build موفق.

جزئیات و محدودیت‌ها در `docs/phase-8.5.md`. cmd/account و cmd/realm deprecated اما
برای سازگاری حفظ شده‌اند؛ مسیر پیشنهادی cmd/wowctl است. فاز ۹ شروع نشده است.

## فاز ۹ — ورود حداقلی به جهان

- `CMSG_PLAYER_LOGIN` با بررسی مالکیت و وضعیت session.
- بسته‌های آغازین: `SMSG_LOGIN_VERIFY_WORLD`، `SMSG_ACCOUNT_DATA_TIMES`، `SMSG_LOGIN_SETTIMESPEED`، tutorial flags، `SMSG_INITIAL_SPELLS`، action buttons، `SMSG_UPDATE_OBJECT` برای خود بازیکن (update fields از دیتابیس).
- time sync، ping و `CMSG_LOGOUT_REQUEST` با ذخیرهٔ وضعیت.

معیار پایان: کاراکتر ایستاده در جهان دیده شود (زمین از MPQ کلاینت) و بتواند تمیز خارج شود.

## فاز ۱۰ — حلقهٔ World و حرکت

- حلقهٔ world تک‌مالک (tick حدود ۵۰ms) برای حالت بازی؛ بدون mutex پخش‌شده.
- پردازش `MSG_MOVE_*` و ذخیرهٔ موقعیت، چت say/yell ساده.

معیار پایان: حرکت و چت پایدار در کلاینت و موقعیت پس از خروج ذخیره شود.

## فاز ۱۱ — حذف کاراکتر و پایدارسازی

- حذف کاراکتر طبق قواعد مرجع (`deleteInfos_*`).
- ورود هم‌زمان و اتصال مجدد، پیام‌های ناقص/نامعتبر/خارج از ترتیب، قطع و ری‌استارت سرورها.
- جدول قابلیت‌های پشتیبانی‌شده و محدودیت‌ها.

معیار پایان: مسیر «ورود ← realm ← فهرست ← ساخت ← ورود به جهان ← خروج ← حذف» با کلاینت واقعی تکرارپذیر باشد.

---

## خارج از محدوده فعلاً

creatureها و grid/visibility، map/vmap/mmap، spell و combat، AI، quest، guild/mail/auction، Warden.

## ریسک‌های شناخته‌شده

- ترتیب بایت و قالب hex در `s`/`v`/`sessionkey` (در فاز ۳.۵ با داده واقعی راستی‌آزمایی می‌شود).
- سازگاری driver Go با MySQL 5.6 و charset (`utf8`).
- ساخت update fields برای `SMSG_UPDATE_OBJECT` سنگین‌ترین بخش پروتکل است (فاز ۹).
- جزئیات ساخت کاراکتر (آیتم‌ها/اسپل‌های اولیه) وابسته به کیفیت داده `mangos1` است.
