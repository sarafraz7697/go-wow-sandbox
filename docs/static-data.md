# فاز ۶ — داده‌های ثابت

از خروجی extractor خود MaNGOS استفاده می‌کنیم. extractor فایل‌ها را از MPQ کلاینت
بیرون می‌کشد؛ loader جدید Go فایل‌های استخراج‌شده را می‌خواند. نوشتن extractor جدید
برای این پروژه لازم نیست.

## فایل‌های مصرفی

مسیر پیش‌فرض `dbc` نسبت به working directory است. فایل‌ها تغییر یا تبدیل نمی‌شوند.

| فایل | fields / record bytes | دادهٔ مصرفی |
|---|---|---|
| ChrRaces.dbc | 69 / 276 | نژاد، faction، مدل زن/مرد، expansion و نام |
| ChrClasses.dbc | 58 / 232 | کلاس، نوع قدرت، خانوادهٔ spell و نام |
| CharBaseInfo.dbc | 2 / 2 | ترکیب‌های نژاد/کلاس مجاز کلاینت |
| CharStartOutfit.dbc | 41 / 152 | نژاد/کلاس/جنسیت/outfit و ۱۲ آیتم اولیه |
| Map.dbc | 125 / 500 | شناسه و نوع نقشه |
| Spell.dbc | 216 / 864 | فقط شناسه‌ها برای بررسی spellهای اولیه و actionها |

فایل‌های دیگر برای این فاز بارگذاری نمی‌شوند. CharSections و داده‌های ظاهر، قبل از
ساخت واقعی کاراکتر در فاز ۸ به مدل اضافه خواهند شد. خواندن Spell.dbc به معنی
پیاده‌سازی منطق spell نیست.

تکمیل فاز ۷: `Item.dbc` و `SpellItemEnchantment.dbc` و Entry/Family از
`creature_template` نیز برای نمایش تجهیزات و pet بارگذاری می‌شوند؛ توضیح در `characters.md`.

فایل‌های فعلی بررسی ساختاری شدند: قالب WDBC، اندازهٔ رکورد Race/Class/Outfit
با TBC منطبق است؛ CharBaseInfo شامل ۵۲ ترکیب است. WDBC شمارهٔ build ندارد؛ سازگاری
قطعی از header به‌تنهایی قابل اثبات نیست. loader علاوه بر layout، وجود Spell 53085
و Map 598 را بررسی می‌کند؛ این دو marker از بررسی نسخهٔ مرجع گرفته شده‌اند.

## بارگذاری و نگهداری

`internal/dbc` طول فایل، header، ضرب اندازهٔ رکوردها، string block و offset رشته‌ها
را کنترل می‌کند. به‌خصوص field_count*4 برای همهٔ فایل‌ها معتبر نیست.
سقف هر فایل ۱۲۸ MiB است. نام‌ها از locale انتخاب‌شده خوانده می‌شوند؛ اگر خالی بود،
اولین ترجمهٔ غیرخالی استفاده می‌شود.

adapter نسخه در `internal/gamedata/tbc` است. ساختار مشترک و getterهای فقط‌خواندنی
در `internal/gamedata` هستند؛ هیچ map یا slice داخلی قابل تغییر بیرون داده نمی‌شود.

از `mangos1` این جدول‌ها فقط هنگام شروع خوانده می‌شوند:

- playercreateinfo و سه جدول item، spell و action مربوط به آن.
- player_levelstats، player_classlevelstats و player_xp_for_level برای سقف سطح ۷۰.
- item_template فقط entry، displayid و InventoryType موردنیاز نمایش/ارجاع.
- reserved_name برای ممنوعیت نام‌های رزروشده بدون حساسیت به حروف.

اطلاعات لباس پیش‌فرض DBC و آیتم‌های اضافهٔ SQL جدا نگهداری می‌شوند؛ یکسان فرض
نمی‌شوند. داده‌های اولیه با Race/Class، Map، Spell و item_template تطبیق داده می‌شوند.
حداقل دادهٔ ضروری ناقص باعث توقف روشن و دارای پیام مشخص می‌شود، پیش از باز شدن listener.

Catalog فقط پس از بارگذاری کامل به handler داده می‌شود. اتصال mangos1 سپس بسته
می‌شود؛ در مسیر درخواست کلاینت هیچ query برای این داده‌های ثابت نداریم. دیتابیس
realmd برای احراز هویت همچنان مستقل است. داده‌های static تا restart ثابت می‌مانند.

بخشی از جدول‌ها MyISAM هستند؛ SELECTهای متوالی snapshot تراکنشی مشترک ندارند.
هنگام startup این جدول‌ها را ویرایش نکن. dump بدون داده مثل all.sql به‌تنهایی کافی نیست.

## تنظیمات و نتیجهٔ قابل مشاهده

در `configs/worldd.conf` اضافه شده است:

```ini
[data]
dbc_path = dbc
dbc_locale = 0
load_timeout = 60s
```

همچنین `[database.world]` برای دیتابیس اصلی `mangos1` روی پورت ۳۳۰۶ تنظیم شده است.
اجرای عادی از ریشهٔ پروژه:

```powershell
go run ./cmd/worldd
```

در startup انتظار لاگ `loading static data` و سپس `static data loaded` با شمارش
race، class، آیتم، ترکیب شروع، آمار سطح و نام رزروشده را داریم. این لاگ هنوز توسط
agent اجرا و مشاهده نشده است. نتیجهٔ تصویری همچنان صفحهٔ کاراکتر خالی فاز ۵ است.

## محدوده و تفاوت با مرجع

- مرجع بعضی ردیف‌های نامعتبر را نادیده می‌گیرد و شکاف آمار سطوح را از سطح قبل
  پر می‌کند. در این پیاده‌سازی دادهٔ ناقص ضروری با خطا متوقف می‌شود؛ آمار جایگزین
  تولید نمی‌کنیم. سطوح بالاتر از ۷۰ و XP سطح صفر/۷۰ به بالا مصرف نمی‌شوند.
- قواعد نام فعلاً زیرمجموعهٔ Latin ASCII، طول ۲ تا ۱۲، بدون فاصله/عدد و با
  normalization حرف اول بزرگ است؛ تمام سیاست‌های locale و strict-mask منگوس
  هنوز پورت نشده‌اند. این API هنوز به CMSG_CHAR_CREATE متصل نیست.
- map type برای نقطهٔ شروع بررسی می‌شود؛ اعتبار هندسی مختصات، zone و ظاهر کاراکتر
  در این مرحله کامل نیست. inventory، spell state و action button واقعی هنوز ساخته نمی‌شوند.
- طبق درخواست کاربر، تست خودکار، build یا اجرای سرور برای این فاز انجام نشده است.

مرجع commit `b2083578a279aac3a70a2cba8fe710a454324c4e`:

- [DBCStructure.h](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/game/Server/DBCStructure.h)
- [DBCStores.cpp](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/game/Server/DBCStores.cpp)
- [ObjectMgrPlayerInfo.cpp](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/game/Object/ObjectMgrPlayerInfo.cpp)
- [ObjectMgrNameCheck.cpp](https://github.com/mangosone/server/blob/b2083578a279aac3a70a2cba8fe710a454324c4e/src/game/Object/ObjectMgrNameCheck.cpp)

## ??? ???? entry ??? ? ?????? ?????

?? ???? item_template ?? entry=0 ?? ??????? ????? ???. ?????? ??? ???????? ????
???????????? ????? ???????? ??? ???? ?? startup ?? ????? ???????. ????? ??? ???????
?? Summary ??? ?????? ? worldd ????? `ignored item_template entry zero` ????????.
????? ?? ???? ????? ?? ??? ?????? ?????. ??? ????? ?? ??????? ??? ?? ????? ???.

???????? ?????: ?? race? ? class? ?? map? ?? ????? ????? ??? outfit? ????? spell?
????? item? ???? spell ?????? ??? action? ???? ???? ???? ???? ??? ???? ????
? ?? ????? XP. ?? ???? ??? ???? ?????? ??. ???? ????? Human/Warrior? Orc/Shaman?
BloodElf/Paladin ? Draenei/Shaman ?? ??? ? ? ?? ????? ????.
