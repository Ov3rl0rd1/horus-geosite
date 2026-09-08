# horus-geosite

Сборка `geosite.dat` для **xray-core** — только домены российских сервисов.

Форк [SagerNet/sing-geosite](https://github.com/SagerNet/sing-geosite),
переделанный с формата sing-box (`.db` / `.srs`) на формат `.dat`, который читает
xray-core.

Источник — `dlc.dat` из последнего релиза
[v2fly/domain-list-community](https://github.com/v2fly/domain-list-community)
(с проверкой sha256). Из него объединяются категории, перечисленные в
[`config.json`](config.json), вычитаются исключения, и результат пишется в
`geosite.dat` двумя категориями:

* `RU` — прямой доступ: `tld-ru` (`.ru`, `.su`, `.рф`, `.москва`, `.tatar`),
  `category-ru` и все отраслевые `category-*-ru` (банки, госуслуги, e-commerce,
  медицина, медиа, образование…), плюс отдельные сервисы: Яндекс, VK, Mail.ru,
  Сбер, Т-Банк, Ozon, Wildberries, Avito, Rutube, Kinopoisk, Okko, 2GIS,
  Kaspersky, МТС, Мегафон, Ростелеком и другие.
* `RU-EXCLUDE` — то, что исключено и должно идти через ВПН. По умолчанию это
  `category-media-ru-blocked` — заблокированные в РФ издания (Meduza,
  Bellingcat, Настоящее Время и т. д.).

## Зачем нужна вторая категория

В формате `.dat` нельзя выразить «этот суффикс, кроме вот этого домена».
Категория `RU` содержит правило на весь домен `.ru`, поэтому заблокированный
`colta.ru` попадает под него, даже если убрать его из списка. Единственный способ
его исключить — отдельное правило маршрутизации **выше** правила `geosite:ru`.
Для этого и существует `RU-EXCLUDE`.

Если правило `geosite:ru-exclude` не добавить в конфиг приложения, исключения
для доменов в зоне `.ru` работать не будут.

## Где брать файл

Ветка `release` обновляется каждой сборкой, ссылка постоянная:

```
https://raw.githubusercontent.com/Ov3rl0rd1/horus-geosite/release/geosite.dat
https://raw.githubusercontent.com/Ov3rl0rd1/horus-geosite/release/geosite.dat.sha256sum
```

Те же файлы прикладываются к GitHub Release с тегом апстрима.

## Как использовать в xray-core

Положите `geosite.dat` в каталог ассетов (`XRAY_LOCATION_ASSET`) рядом с
`geoip.dat` из [horus-geoip](https://github.com/Ov3rl0rd1/horus-geoip):

```json
{
  "routing": {
    "domainStrategy": "IPIfNonMatch",
    "rules": [
      { "type": "field", "domain": ["geosite:ru-exclude"], "outboundTag": "proxy" },
      { "type": "field", "domain": ["geosite:ru"], "outboundTag": "direct" },
      { "type": "field", "ip": ["geoip:ru"], "outboundTag": "direct" },
      { "type": "field", "network": "tcp,udp", "outboundTag": "proxy" }
    ]
  }
}
```

Порядок правил важен: `ru-exclude` строго выше `ru`.

Атрибуты апстрима сохраняются, так что работают и выборки вида `geosite:ru@ads`.

## config.json

| Поле | Значение |
| --- | --- |
| `source_repository` | Репозиторий `owner/name`, из последнего релиза которого берётся список. |
| `source_asset` | Имя ассета релиза (`dlc.dat`). |
| `verify_checksum` | Сверять ассет с соседним `<asset>.sha256sum`. |
| `output_file` | Имя выходного файла. |
| `category` | Код «прямой» категории. Пишется в верхнем регистре, в конфиге xray — `geosite:ru`. |
| `exclude_category` | Код категории исключений. Пустая строка — не писать её вовсе. |
| `include_categories` | Коды апстрима, объединяемые в `category`. Несуществующий код — ошибка сборки, так опечатки и переименования в апстриме ловятся сразу. |
| `exclude_categories` | Коды апстрима, чьи домены вычитаются и попадают в `exclude_category`. |
| `exclude_attributes` | Выбросить домены с такими атрибутами апстрима, например `["ads"]`. |
| `include` | Дополнительные правила в `category`. |
| `exclude` | Правила исключения. Исключение сильнее всего остального, включая `include`. |

### Синтаксис правил в `include` / `exclude`

Тот же, что в правилах маршрутизации xray:

| Запись | Что совпадает |
| --- | --- |
| `example.ru` или `domain:example.ru` | Сам домен и все поддомены. `suffix:` — синоним. |
| `full:www.example.ru` | Только точное имя. |
| `keyword:example` | Любое имя, содержащее подстроку. |
| `regexp:^ad[0-9]+\.example\.ru$` | Любое имя, подходящее под регулярное выражение. |

Пример:

```json
{
  "include": ["domain:my-service.ru", "full:api.partner.com"],
  "exclude": ["domain:mail.ru", "keyword:adfox", "regexp:^ads?\..*\.ru$"],
  "exclude_attributes": ["ads"]
}
```

Правило `domain:` при исключении убирает и поддомены: `domain:mail.ru` снимает
и `smtp.mail.ru`. Правило `full:` намеренно не трогает более широкую запись
`domain:` с тем же именем — иначе вместе с одним хостом отвалились бы все его
поддомены; такой случай закрывается категорией `ru-exclude`.

## Сборка

```bash
go run .            # собрать geosite.dat по config.json
go test ./...       # тесты правил исключения и разбора конфига
make lint           # golangci-lint
```

Переменные окружения:

| Переменная | Назначение |
| --- | --- |
| `CONFIG` | Путь к конфигу, по умолчанию `config.json`. |
| `FIXED_RELEASE` | Тег апстрим-релиза вместо последнего. |
| `ACCESS_TOKEN` | Токен GitHub, если упирается в лимит API. |

## CI

* `build.yaml` — на каждый push и PR в `main`: тесты, сборка, артефакт.
* `release.yaml` — ежедневно и по кнопке: сборка, sha256, push в ветку
  `release`, GitHub Release с тегом апстрима.

Проверки «а не собирали ли уже этот релиз» нет — сборка запускается всегда,
иначе правка `config.json` не попала бы в выпуск при неизменившемся апстриме.
Тег релиза равен тегу апстрима, так что повторный прогон обновляет тот же релиз,
а не плодит новые.
