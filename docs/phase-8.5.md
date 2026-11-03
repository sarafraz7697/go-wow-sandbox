# فاز ۸٫۵ — مرزبندی پیش از ورود به جهان

پیاده‌سازی و بررسی خودکار در ۲۰۲۶-۱۱-۰۳ انجام شد. دامنهٔ قابلیت‌های بازی همچنان
فهرست کاراکتر و ساخت Human Warrior است؛ ورود به جهان و حذف فعال نشده‌اند.

## Session و مالکیت

بعد از handshake، هر اتصال یک reader، یک حلقهٔ session و یک writer دارد.
reader مالک Reader و رمزنگاری ورودی است؛ صف ورودی ظرفیت ۱ دارد. حلقهٔ session
تنها مالک تغییر state است و packetها و رویدادهای داخلی را به‌ترتیب پردازش می‌کند.
writer مالک Writer و رمزنگاری خروجی است؛ صف خروجی همان send_queue تنظیم‌شده است.
در shutdown، اتصال بسته و پایان reader و writer انتظار کشیده می‌شود.

حالت‌ها: Authed، LoggingIn، InWorld، LoggingOut و Closed. مسیرهای مجاز:

- Authed → LoggingIn؛ LoggingIn → InWorld یا Authed در شکست ورود.
- InWorld → LoggingOut؛ LoggingOut → Authed یا InWorld در لغو خروج.
- هر حالت زنده → Closed؛ Closed پایانی است.

در فاز فعلی اتصال پس از handshake در Authed می‌ماند. حالت‌های دیگر زیرساخت فازهای
بعد هستند، نه ادعای ورود پیاده‌شده به جهان. worker آینده فقط RequestTransition
می‌فرستد و نباید state را مستقیم بخواند یا بنویسد. بعد از enqueue منتظر نتیجه یا
shutdown می‌ماند؛ متد نباید از خود حلقهٔ session فراخوانی شود.
حلقه هنگام انتظار socket می‌تواند رویداد دریافت کند، اما handlerهای فعلی هنوز
همگام‌اند؛ I/O طولانی handler باید پیش از اتصال world loop طراحی مجدد شود.

Router یک allowlist صریح است. CHAR_ENUM/CREATE/DELETE/PLAYER_LOGIN/REALM_SPLIT فقط
در Authed و PING/KEEP_ALIVE در حالت‌های زنده مجازند. opcode ثبت‌نشده رد می‌شود؛
metadata تولیدشدهٔ MaNGOS به‌تنهایی مجوز اجرا نیست. active با mutex فقط admission
حساب‌هاست و مالک وضعیت بازیکن محسوب نمی‌شود.

## Codec و ساخت

`protocol/world/tbc/codec` شامل auth payload، addon، پاسخ‌های سادهٔ صفحهٔ کاراکتر،
decode ساخت و encode کاراکتر است. این پکیج socket یا SQL ندارد. framing و crypto
در `protocol/world/tbc` باقی‌اند؛ رفتار handshake و transport تغییر نکرده است.

`character.Service` درخواست را به CreationProfile می‌دهد و plan معتبر را به store
می‌فرستد. `character.CreationPolicy.Check` روی CreationFacts تصمیم می‌گیرد. policy
در startup مقداردهی و هنگام ساخت Creator تزریق می‌شود؛ callback اختیاری هر درخواست
وجود ندارد. storage facts را زیر GET_LOCK و transaction می‌خواند، policy را همان‌جا
اجرا می‌کند و فقط پس از موفقیت می‌نویسد. بررسی session و ban مستقل باقی مانده‌اند.

قالب ۶۰ کلمه‌ای آیتم در `storage/characters/mangosone8606` قرار دارد. adapter ذخیره‌سازی
فعلی همچنان schema اصلی MaNGOS One را هدف می‌گیرد؛ برای نسخهٔ دیگر باید adapter
مناسب انتخاب شود. بهینه‌سازی INSERT گروهی و transaction مشترک شمارنده حفظ شده‌اند.

## راه‌اندازی و تنظیمات

`app.RunRealmd` و `app.RunWorldd` مالک راه‌اندازی و بستن منابع هستند؛ mainها parse،
signal و گزارش خطای خروج را انجام می‌دهند. composition root فعلی صریحاً protocol،
DBC و persistence نسخهٔ 8606 را متصل می‌کند و build/persistence را لاگ می‌کند.
انتخاب نسخه در runtime هنوز اضافه نشده است.

`RealmdConfig` فقط Common دارد؛ `WorldConfig` تنظیمات دیتابیس‌های world/characters و
DBC و realm را اضافه می‌کند. realmd validation معنایی world را انجام نمی‌دهد؛
syntax فایل همچنان strict است. Config/Parse قدیمی برای سازگاری ابزارهای admin و
parser مشترک باقی مانده‌اند. فایل‌های conf و تقدم CLI/env/file تغییر نکرده‌اند.
cmd/account و cmd/realm wrapperهای deprecated اما فعال‌اند؛ ابزار پیشنهادی wowctl است.

## شواهد بررسی

- تست‌های موجود پیش از بازآرایی پاس شدند.
- `go test -race ./...` با تست integration ساخت روی MySQL آزمایشی پاس شد.
- `go vet ./...` و `go build ./cmd/...` پاس شدند.
- تست انتقال‌های مجاز/غیرمجاز، رد opcode پیش از handler و پردازش رویداد هنگام سکوت
  reader اضافه شد؛ تست‌های timeout، revocation و shutdown موجود نیز پاس شدند.
- تست policy، سرویس ساخت، validation مستقل config و بردار قالب آیتم اضافه شد.
- رقابت دو حساب روی یک نام: دقیقاً یک موفقیت؛ رقابت آخرین جایگاه realm: یک موفقیت
  و یک خطای سقف؛ شمارنده درست باقی ماند.
- خطای درج میانی و خطای تزریق‌شده در آخرین نوشتن realmcharacters: rollback بدون
  دادهٔ نیمه‌کاره؛ اتصال و قفل برای درخواست بعدی قابل استفاده ماندند.
- سقف کل حساب و session با کلید تغییرکرده نیز بررسی شدند.

تست integration فقط با WOW_CREATION_TEST_DSN روی 127.0.0.1:3308 فعال می‌شود.
از schemaهای sandbox، جدول‌های خالی در schemaهای موقت wow_test_creation_* می‌سازد
و در پایان همان schemaها را پاک می‌کند. دیتابیس اصلی پورت ۳۳۰۶ تغییر نکرده است.
تست بدون این متغیر integration را skip می‌کند و نباید آن را تأیید SQL تلقی کرد.
تأیید کلاینت اصلی مربوط به فازهای ۷ و ۸ است؛ نسخهٔ بازآرایی‌شده هنوز با کلاینت
واقعی توسط کاربر بررسی نشده است.

اجرای همان سرویس‌ها کافی است؛ تنظیم جدید یا migration لازم نیست:

```powershell
go run ./cmd/realmd
go run ./cmd/worldd
```
