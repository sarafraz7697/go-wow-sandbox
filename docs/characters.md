# فاز ۷ — فهرست کاراکترهای حساب

وضعیت: کاربر فاز ۷ را با کلاینت اصلی تأیید کرده است. تست خودکار بیشتری اجرا نشده است.
ساخت کاراکتر در فاز ۸ اضافه شده؛ راهنمای فعلی در [ساخت کاراکتر](character-creation.md) است.

## رفتار

پس از احراز هویت World، درخواست CMSG_CHAR_ENUM از دیتابیس `character1` فهرست حساب
احرازشده را می‌خواند. account ID از session می‌آید، نه از ورودی کلاینت. درخواست enum
باید بدنهٔ خالی داشته باشد. قبل و بعد از خواندن، session key و دسترسی حساب دوباره بررسی می‌شوند.

فیلدهای خوانده‌شده شامل GUID، نام، race/class/gender، level، playerBytes، playerBytes2،
zone/map، مختصات، guild، playerFlags، at_login، pet جاری و equipmentCache هستند.
رکوردهای دارای deleteDate از فهرست حذف می‌شوند و ترتیب با GUID است.

هر process به CharacterDSN همان Realm متصل است؛ جدول characters ستون realm_id ندارد.
برای Realm دوم باید تنظیمات/دیتابیس مناسب همان Realm را داد. مسیر enumeration فقط‌خواندنی
است؛ تغییر کاراکتر و شمارندهٔ realmcharacters مربوط به مسیر ساخت فاز ۸ است.

## بستهٔ TBC

- تعداد کاراکترها یک بایت؛ GUID کامل ۸ بایتی است، نه packed GUID.
- ظاهر از بایت‌های playerBytes و facial hair از بایت پایین playerBytes2 استخراج می‌شود.
- hide helm/cloak، ghost، rename و first login مطابق مرجع نگاشت می‌شوند.
- حالت declined names مانند تنظیم پیش‌فرض غیرفعال مرجع است؛ flag متناظر ارسال می‌شود.
- برای ۱۹ جایگاه تجهیزات، displayID، inventory type و enchant visual نوشته می‌شود.
  سپس جایگاه خالی first bag نوشته می‌شود؛ هر جایگاه ۹ بایت است.
- cache شامل زوج item entry و packed enchants است. نیمهٔ پایین enchant دائمی و
  نیمهٔ بالای آن موقت است؛ اولین شناسهٔ موجود در DBC انتخاب می‌شود.
- pet فقط برای Hunter/Warlock غیر ghost نمایش داده می‌شود؛ family از creature_template
  و مدل/سطح از character_pet با slot=0 می‌آید.

## داده‌های اضافهٔ startup

`Item.dbc` با layout چهار فیلد/۱۶ بایت و `SpellItemEnchantment.dbc` با ۳۴ فیلد/۱۳۶ بایت
به loader اضافه شدند. displayID آیتم موجود در Item.dbc مطابق مرجع تصحیح می‌شود؛
InventoryType دیتابیس حفظ می‌شود. از creature_template فقط Entry و Family خوانده می‌شود.
این داده‌ها در Catalog فقط‌خواندنی قرار دارند؛ query به mangos1 هنگام CHAR_ENUM نداریم.

برای equipment از equipmentCache استفاده می‌شود، همان مسیر MaNGOS. fallback به
character_inventory یا بازسازی cache فعلاً نداریم. cache خالی/کوتاه به تجهیزات خالی
منجر می‌شود؛ اگر cache در دیتابیس قدیمی باشد نمایش نیز همان snapshot قدیمی خواهد بود.

## خطاها و محدودیت‌ها

- نام/موقعیت نامعتبر، ترکیب نژاد/کلاس بدون تعریف یا cache غیرعددی باعث حذف همان کاراکتر
  از پاسخ با هشدار GUID می‌شود. تعداد نهایی فقط کاراکترهای سریال‌شده است.
- آیتم ناشناخته به جایگاه خالی تبدیل می‌شود؛ enchant ناشناخته visual صفر دارد.
- وجود بیش از ۲۵۵ رکورد یا پاسخ بزرگ‌تر از سقف framing خطاست؛ داده بی‌صدا truncate نمی‌شود.
- اگر دادهٔ قدیمی چند pet جاری داشته باشد، کمترین pet ID انتخاب می‌شود تا کاراکتر تکراری نشود.
- خطای دیتابیس اتصال را می‌بندد؛ به‌اشتباه فهرست خالی موفق ارسال نمی‌شود.
- این فاز فقط نمایش است؛ ساخت در فاز ۸ اضافه شده و حذف و Enter World هنوز غیرفعال‌اند.
- fixture در دیتابیس واقعی ساخته نشده؛ کاربر نمایش را با کلاینت اصلی تأیید کرده است.

## اجرا

بخش `[database.characters]` در `configs/worldd.conf` برای `character1` روی پورت ۳۳۰۶
اضافه شده است. پس از restart معمولی:

```powershell
go run ./cmd/worldd
```

لاگ startup فعلی `phase=8` و `character database connected` دارد. با بازشدن صفحهٔ انتخاب کاراکتر،
لاگ debug با پیام `character list prepared` تعداد قابل‌نمایش و omitted را می‌نویسد.
اگر حساب واقعاً کاراکتری ندارد، فهرست همچنان خالی خواهد بود.

مرجع بسته: [Player::BuildEnumData](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/game/Object/Player.cpp).
