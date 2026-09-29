# -*- coding: utf-8 -*-
from reportlab.pdfgen import canvas
from reportlab.pdfbase import pdfmetrics
from reportlab.pdfbase.ttfonts import TTFont
from reportlab.lib.pagesizes import A4
from reportlab.lib.colors import Color, white

pdfmetrics.registerFont(TTFont("Hei", "/System/Library/Fonts/STHeiti Medium.ttc", subfontIndex=0))
pdfmetrics.registerFont(TTFont("HeiL", "/System/Library/Fonts/STHeiti Light.ttc", subfontIndex=0))
FONT = "Hei"
LIGHT = "HeiL"

OUT = "/Users/sunqirui/gitlab/aiagent/xhub/output/pdf/image-pricing.pdf"
W, H = A4

INK = Color(0.11, 0.13, 0.17)
MUTED = Color(0.42, 0.46, 0.52)
LINE = Color(0.90, 0.92, 0.94)
HAIR = Color(0.93, 0.94, 0.95)

ORANGE = Color(0.91, 0.36, 0.18)
ORANGE_BG = Color(1.0, 0.96, 0.94)
TEAL = Color(0.08, 0.55, 0.52)
TEAL_BG = Color(0.92, 0.98, 0.96)
INDIGO = Color(0.28, 0.34, 0.62)
INDIGO_BG = Color(0.94, 0.95, 0.98)
GROK = Color(0.12, 0.45, 0.28)
GROK_BG = Color(0.93, 0.98, 0.94)
CHIP = Color(0.965, 0.972, 0.978)

gpt = [
    ("1K", "0.15", ORANGE, ORANGE_BG, [
        "1024 x 1024", "1152 x 864", "864 x 1152", "1536 x 1024",
        "1024 x 1536", "1344 x 768", "768 x 1344", "1536 x 656",
        "1120 x 896", "896 x 1120",
    ]),
    ("2K", "0.20", TEAL, TEAL_BG, [
        "2048 x 2048", "2048 x 1536", "1536 x 2048", "2304 x 1536",
        "1536 x 2304", "2048 x 1152", "1152 x 2048", "2304 x 992",
        "1920 x 1536", "1536 x 1920",
    ]),
    ("4K", "0.30", INDIGO, INDIGO_BG, [
        "2880 x 2880", "3200 x 2400", "2400 x 3200", "3456 x 2304",
        "2304 x 3456", "3840 x 2160", "2160 x 3840", "3840 x 1648",
        "3200 x 2560", "2560 x 3200",
    ]),
]

grok_rows = [
    ("1K", "0.15"),
    ("2K", "0.20"),
]

c = canvas.Canvas(OUT, pagesize=A4)
c.setTitle("GPT Image 2.0 / 2.5 与 Grok Image 2.0 价格")
c.setAuthor("xhub")

c.setFillColor(white)
c.rect(0, 0, W, H, fill=1, stroke=0)

# header
c.setFillColor(INK)
c.setFont(FONT, 22)
c.drawString(28, H - 48, "GPT Image 2.0 / 2.5")
c.setFillColor(MUTED)
c.setFont(LIGHT, 11)
c.drawString(28, H - 70, "图片生成价格参考")
c.setFont(LIGHT, 8.5)
c.drawRightString(W - 28, H - 46, "按次计费")
c.drawRightString(W - 28, H - 60, "价格单位：人民币")

c.setStrokeColor(HAIR)
c.setLineWidth(0.8)
c.line(28, H - 86, W - 28, H - 86)

margin = 22
gap = 10
card_w = (W - margin * 2 - gap * 2) / 3.0
card_top = H - 104
card_h = 430


def rounded_header(x, y, w, h, color):
    c.setFillColor(color)
    c.roundRect(x, y, w, h, 8, fill=1, stroke=0)
    c.rect(x, y, w, 10, fill=1, stroke=0)


for i, (name, price, accent, bg, sizes) in enumerate(gpt):
    x = margin + i * (card_w + gap)
    y = card_top - card_h
    c.setFillColor(white)
    c.setStrokeColor(LINE)
    c.setLineWidth(0.8)
    c.roundRect(x, y, card_w, card_h, 10, fill=1, stroke=1)

    rounded_header(x + 0.4, y + card_h - 62, card_w - 0.8, 62, bg)

    # badge
    c.setFillColor(accent)
    c.circle(x + 26, y + card_h - 32, 13, fill=1, stroke=0)
    c.setFillColor(white)
    c.setFont(FONT, 8)
    c.drawCentredString(x + 26, y + card_h - 35, name)

    c.setFillColor(INK)
    c.setFont(FONT, 12)
    c.drawString(x + 46, y + card_h - 28, name + " 分辨率")
    c.setFillColor(accent)
    c.setFont(FONT, 11)
    c.drawString(x + 46, y + card_h - 46, price + " 元/次")

    c.setFillColor(MUTED)
    c.setFont(LIGHT, 7.5)
    c.drawString(x + 14, y + card_h - 80, "支持尺寸（宽 x 高，像素）")

    row_h = 32.2
    box_h = 24
    start = y + card_h - 108
    for n, size in enumerate(sizes):
        sy = start - n * row_h
        c.setFillColor(CHIP)
        c.roundRect(x + 12, sy, card_w - 24, box_h, 5, fill=1, stroke=0)
        c.setFillColor(accent)
        c.circle(x + 24, sy + box_h / 2.0, 2.2, fill=1, stroke=0)
        c.setFillColor(INK)
        c.setFont(FONT, 8.5)
        c.drawString(x + 34, sy + 7.5, size)

# grok band
band_h = 118
band_y = card_top - card_h - 16 - band_h
c.setFillColor(GROK_BG)
c.setStrokeColor(Color(0.78, 0.90, 0.82))
c.setLineWidth(0.9)
c.roundRect(margin, band_y, W - margin * 2, band_h, 10, fill=1, stroke=1)

c.setFillColor(GROK)
c.roundRect(margin + 14, band_y + band_h - 32, 62, 16, 8, fill=1, stroke=0)
c.setFillColor(white)
c.setFont(FONT, 8)
c.drawCentredString(margin + 45, band_y + band_h - 27, "可过风控")

c.setFillColor(INK)
c.setFont(FONT, 13)
c.drawString(margin + 86, band_y + band_h - 28, "Grok Image 2.0")
c.setFillColor(MUTED)
c.setFont(LIGHT, 8)
c.drawString(margin + 202, band_y + band_h - 26, "grok-imagine-image-2.0    文生图 / 图生图")

facts = [
    ("输入图片", "0.024 元/张"),
    ("1K 输出", "0.15 元/张"),
    ("2K 输出", "0.20 元/张"),
]
inner = W - margin * 2 - 28
fact_gap = 8
fact_w = (inner - fact_gap * 2) / 3.0
fact_h = 48
fact_y = band_y + 16
for i, (label, value) in enumerate(facts):
    fx = margin + 14 + i * (fact_w + fact_gap)
    c.setFillColor(white)
    c.roundRect(fx, fact_y, fact_w, fact_h, 7, fill=1, stroke=0)
    c.setFillColor(GROK)
    c.setFont(FONT, 8)
    c.drawString(fx + 12, fact_y + 30, label)
    c.setFillColor(INK)
    c.setFont(FONT, 12)
    c.drawString(fx + 12, fact_y + 12, value)

c.setFillColor(MUTED)
c.setFont(LIGHT, 7.5)
c.drawString(28, 28, "说明：GPT Image 按分辨率档位计次。Grok 输入与输出分开计价，可过风控。具体以实际服务页面为准。")
c.drawRightString(W - 28, 28, "1 / 1")

c.showPage()
c.save()
print("wrote", OUT)
