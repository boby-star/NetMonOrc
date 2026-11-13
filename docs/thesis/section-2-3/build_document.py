"""Build the standalone thesis subsection; requires pandoc, dot and python-docx."""
from pathlib import Path
import re
import subprocess
from docx import Document
from docx.shared import Cm, Pt
from docx.shared import RGBColor
from docx.enum.text import WD_ALIGN_PARAGRAPH
from docx.oxml import OxmlElement
from docx.oxml.ns import qn

ROOT = Path(__file__).resolve().parent
for stem in ['adaptive-flow', 'lab-topology']:
    subprocess.run(['dot', '-Tpng', '-Gdpi=200', str(ROOT/'assets'/f'{stem}.dot'),
                    '-o', str(ROOT/'assets'/f'{stem}.png')], check=True)

text = (ROOT/'section-2-3.md').read_text()
text = re.sub(r'\\tag\{([\d.]+)\}', r'\\qquad (\1)', text)
text = text.replace('<!-- PAGE -->', '')
temp = ROOT/'_typeset.md'
temp.write_text(text)
subprocess.run(['pandoc', str(temp), '-f', 'markdown+raw_attribute', '-t', 'docx',
                '--resource-path', str(ROOT), '-o', str(ROOT/'section-2-3.docx')], check=True)
temp.unlink()

doc = Document(ROOT/'section-2-3.docx')
sec = doc.sections[0]
sec.page_width, sec.page_height = Cm(21), Cm(29.7)
sec.top_margin, sec.bottom_margin = Cm(2), Cm(2)
sec.left_margin, sec.right_margin = Cm(3), Cm(1.5)
sec.header_distance, sec.footer_distance = Cm(0.8), Cm(1)

for stylename in ['Normal', 'Body Text', 'First Paragraph']:
    style = doc.styles[stylename]
    style.font.name, style.font.size = 'Times New Roman', Pt(14)
    style.paragraph_format.line_spacing = 1.15
    style.paragraph_format.space_after = Pt(0)
    style.paragraph_format.space_before = Pt(0)
    style.paragraph_format.first_line_indent = Cm(1.25)
    style.paragraph_format.alignment = WD_ALIGN_PARAGRAPH.JUSTIFY

for p in doc.paragraphs:
    pf = p.paragraph_format
    pf.widow_control = True
    if p.style.name.startswith('Heading'):
        p.style.font.name, p.style.font.size = 'Times New Roman', Pt(14)
        p.style.font.color.rgb = RGBColor(0, 0, 0)
        for r in p.runs:
            r.font.name, r.font.size = 'Times New Roman', Pt(14)
            r.font.color.rgb = RGBColor(0, 0, 0)
        pf.first_line_indent = Cm(0)
        pf.space_after, pf.space_before = Pt(6), Pt(0)
        pf.line_spacing = 1.3
    if p.style.name == 'Source Code':
        pf.first_line_indent = Cm(0)
        pf.line_spacing = 1
        pf.space_after = Pt(0)
        pf.keep_together = True
        pf.alignment = WD_ALIGN_PARAGRAPH.LEFT
        for r in p.runs:
            r.font.name, r.font.size = 'DejaVu Sans Mono', Pt(9.5)
    if p._p.xpath('.//m:oMath'):
        pf.first_line_indent = Cm(0)
        pf.line_spacing = 1.1
        pf.space_before, pf.space_after = Pt(5), Pt(5)
        pf.alignment = WD_ALIGN_PARAGRAPH.CENTER
        # Word mathematical objects remain editable; size their runs explicitly.
        for mathrun in p._p.xpath('.//m:r'):
            rpr = mathrun.find(qn('w:rPr'))
            if rpr is None:
                rpr = OxmlElement('w:rPr')
                mathrun.insert(0, rpr)
            size = OxmlElement('w:sz')
            size.set(qn('w:val'), '24')
            rpr.append(size)
    if p.text.startswith(('Таблиця 2.', 'Алгоритм 2.')):
        pf.first_line_indent = Cm(0)
        pf.line_spacing = 1.1
        pf.space_before, pf.space_after = Pt(4), Pt(4)
        pf.keep_with_next = True
        for r in p.runs:
            r.font.name, r.font.size = 'Times New Roman', Pt(12)
    if p.style.name in ['Image Caption', 'Caption']:
        pf.first_line_indent = Cm(0)
        pf.line_spacing = 1.1
        pf.space_after = Pt(6)
        pf.alignment = WD_ALIGN_PARAGRAPH.CENTER
        for r in p.runs:
            r.font.name, r.font.size = 'Times New Roman', Pt(12)
    if p._p.xpath('.//w:drawing'):
        pf.first_line_indent = Cm(0)
        pf.line_spacing = 1
        pf.alignment = WD_ALIGN_PARAGRAPH.CENTER
        pf.keep_with_next = True

for i, shape in enumerate(doc.inline_shapes):
    target_w = Cm(15)
    ratio = shape.height/shape.width
    shape.width, shape.height = target_w, int(target_w*ratio)

widths = [[4,12.5], [6,10.5], [7.5,1.5,1.5,1.5,1.5,1.5,1.5],
          [1.5,7,3,5], [3.5,4.33,4.33,4.34]]
for table, dimensions in zip(doc.tables, widths):
    table.autofit = False
    for col, width in zip(table.columns, dimensions):
        col.width = Cm(width)
    for row_idx, row in enumerate(table.rows):
        trpr = row._tr.get_or_add_trPr()
        trpr.append(OxmlElement('w:cantSplit'))
        if row_idx == 0:
            trpr.append(OxmlElement('w:tblHeader'))
        for cell, width in zip(row.cells, dimensions):
            cell.width = Cm(width)
            for p in cell.paragraphs:
                pf = p.paragraph_format
                pf.first_line_indent = Cm(0)
                pf.line_spacing = 1
                pf.space_before, pf.space_after = Pt(2), Pt(2)
                pf.alignment = WD_ALIGN_PARAGRAPH.LEFT
                for r in p.runs:
                    r.font.name, r.font.size = 'Times New Roman', Pt(11)

footer = sec.footer.paragraphs[0]
footer.alignment = WD_ALIGN_PARAGRAPH.CENTER
field = OxmlElement('w:fldSimple')
field.set(qn('w:instr'), 'PAGE')
footer._p.append(field)
doc.core_properties.title = '2.3 Розроблення алгоритму адаптивного виконання діагностичних перевірок'
doc.core_properties.subject = 'NetMonOrc — підрозділ магістерської роботи'
doc.core_properties.language = 'uk-UA'
doc.save(ROOT/'section-2-3.docx')
print('Document saved:', ROOT/'section-2-3.docx')
print('Words in Markdown:', len(text.split()))
