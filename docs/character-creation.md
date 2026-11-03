# فاز ۸ — ساخت کاراکتر

وضعیت: پیاده‌سازی شده؛ تست، build و اجرای سرور انجام نشده است. فاز ۷ را کاربر با کلاینت اصلی تأیید کرده است.

## محدوده

ساخت Human Warrior با هر دو جنسیت، سطح ۱ و پول صفر فعال است. ترکیب‌های دیگر پاسخ
`CHAR_CREATE_DISABLED` می‌گیرند. ساخت موفق در درخواست بعدی CHAR_ENUM نمایش داده می‌شود.
Enter World و حذف کاراکتر همچنان مربوط به فازهای بعد هستند.

نام فعلاً فقط حروف انگلیسی، ۲ تا ۱۲ حرف، بدون سه حرف یکسان متوالی است؛ نام رزروشده
و تکراری رد می‌شود. این سیاست شامل حساب GM هم می‌شود. حروف نام به شکل Title Case
ذخیره می‌شوند. فیلتر profanity و قواعد زبان‌های دیگر هنوز وجود ندارند.

ظاهر از CharSections و CharacterFacialHairStyles بررسی می‌شود. شناسهٔ outfit ارسالی
مانند مرجع نادیده گرفته می‌شود؛ outfit اولیهٔ همین ترکیب از DBC انتخاب می‌شود.
موقعیت، stats، spellها، action bar و آیتم‌های اضافه از Catalog فاز ۶ می‌آیند.
مهارت‌ها از SkillLine/SkillLineAbility و reputation اولیه از Faction گرفته می‌شوند.
reputation ذخیره‌شده offset صفر نسبت به مقدار پایهٔ DBC است.

این initializer برای همین ترکیب و وسایل معمول شروع نوشته شده؛ سیستم عمومی equip،
bag، random enchant یا اجرای spell نیست. دادهٔ شروع با آیتم یا effect پشتیبانی‌نشده
خطا می‌دهد. برای فعال‌کردن ترکیب بعدی باید قواعدش اضافه شوند، نه فقط شرط race/class.

## ذخیره‌سازی

از schema موجود استفاده می‌شود؛ migration یا جدول سفارشی اضافه نشده است.
در یک transaction این موارد ثبت می‌شوند:

- characters و character_homebind؛ appearance، health، taxi mask و equipmentCache.
- item_instance با قالب ۶۰ کلمه‌ای Update Fields نسخهٔ 8606 و character_inventory.
- character_spell، character_action، character_skills و character_reputation.
- شمارندهٔ realmcharacters در دیتابیس login.

login و characters باید روی یک endpoint MySQL باشند؛ نام دیتابیس login از DSN خوانده
می‌شود، بنابراین realmd در کد ثابت نیست. کاربر اتصال characters باید به جدول‌های لازم
در login نیز دسترسی داشته باشد. جدول‌های نوشته‌شونده باید InnoDB باشند و startup این
شرط را بررسی می‌کند. تنظیمات فعلی root روی پورت ۳۳۰۶ با این مدل سازگارند.

قفل GET_LOCK روی یک connection اختصاصی، بررسی نام و تخصیص GUID را بین processهای Go
همان دیتابیس سریالی می‌کند. account نیز داخل تراکنش قفل می‌شود. **سرور C++ را هم‌زمان
برای نوشتن روی همین character database اجرا نکنید**؛ آن سرور این قفل قراردادی Go را
نمی‌گیرد. راه‌اندازی realmd و worldd خود پروژه هم‌زمان مجاز است.

سقف فعلی ۱۰ کاراکتر در realm و ۵۰ کاراکتر در کل حساب است. شمارش realm جاری از characters
و سایر realmها از realmcharacters گرفته می‌شود. دادهٔ قدیمی سایر realmها باید شمارندهٔ
درست داشته باشد. روی realm با icon برابر ۱ یا ۶ وجود کاراکتر Horde مانع ساخت Human است.
این محدودیت فعلاً برای GM هم اعمال می‌شود.

قبل از نوشتن، session key، قفل IP، ban و دسترسی realm دوباره بررسی می‌شوند. موفقیت فقط
بعد از commit ارسال می‌شود. خطای commit خودکار retry نمی‌شود؛ اگر اتصال هنگام commit
قطع شود، ممکن است نتیجه برای سرور نامعلوم باشد و باید فهرست کاراکتر دوباره خوانده شود.

## اجرا و لاگ

همان `configs/worldd.conf` و فولدر `dbc` استفاده می‌شوند:

```powershell
go run ./cmd/worldd
```

startup باید `phase=8` داشته باشد. با debug، `creating character` و خطای اعتبارسنجی
ثبت می‌شود؛ بعد از commit پیام `character created` همراه account_id و GUID چاپ می‌شود.
رمز عبور و session key لاگ نمی‌شوند. این تغییر خودش هیچ کاراکتری در دیتابیس نساخته است.

برای کاهش رفت‌وبرگشت دیتابیس، spellها، actionها، skillها، reputationها و آیتم‌ها با
INSERT چندردیفی (حداکثر ۱۲۸ ردیف در هر دستور) در همان transaction ذخیره می‌شوند.
پارامترها همچنان bind می‌شوند؛ multiStatements فعال نشده است. برای دادهٔ فعلی، مسیر
موفق ۹ دستور نوشتن دارد؛ SELECTها و کنترل تراکنش جدا هستند.
در سطح debug، `character creation stage` فیلدهای `stage` و `elapsed_ms` را چاپ می‌کند:
`advisory_lock` انتظار قفل، `account_lock_and_session` بررسی حساب، `allocate_guids`
تخصیص شناسه، `write_*` درج‌ها و `commit` ثبت تراکنش هستند. پیام
`character creation storage finished` زمان کل storage و `write_statements` را دارد.
عدد ۵ در GET_LOCK حداکثر انتظار برای قفل مشغول است؛ تأخیر ثابت برای هر ساخت نیست.
بهینه‌سازی با اجرای سرور یا تست دیتابیس هنوز اندازه‌گیری نشده است.

## مرزبندی نسخه‌ها

decoder و کدهای پاسخ در `internal/protocol/world/tbc`، قواعد و layoutهای DBC در
`internal/character/tbc` و snapshot مستقل از شبکه در `internal/character` هستند.
storage فعلی adapter دیتابیس MaNGOS One است؛ قالب item_instance آن مختص 8606 است.
نسخهٔ بعدی به adapter خودش نیاز دارد و نباید همین قالب را برای آن استفاده کرد.

مرجع: MaNGOS One commit `b2083578a279aac3a70a2cba8fe710a454324c4e`،
Player::Create، PlayerTaxi، ReputationMgr و UpdateFields. پشتیبانی کامل سرور C++ ادعا نمی‌شود.
