import re
from sys import argv
from glob import glob
from os.path import basename, join, abspath, splitext, expanduser
from os import makedirs
import subprocess
from time import time

pathList = []
for i, a in enumerate(argv[1:]):
	paths = glob(a)
	pathList.extend(paths)
	# print('%3d: %3d %3d %s' % (i, len(paths), len(pathList), paths))
print('Processing %d files' % len(pathList))

poplDir = 'popl'
poplExe = expanduser('~/pdf/poppler/build/utils/pdftotext')
poplExe = 'pdftotext'
uniExe = './pdf_tables_text' # './pdf_extract_text'
makedirs(poplDir, exist_ok=True)

numPages = '10'
badFiles = []
t0 = time()
doPoppler = True
for i, pdfPath in enumerate(pathList):
	base = basename(pdfPath)
	base, _ = splitext(base)
	textPath = join(poplDir, '%s.txt' % base)
	print('%4d of %d: %40s -- %s' % (i, len(pathList), pdfPath, textPath))
	s = subprocess.run([uniExe, '-l', numPages, pdfPath],  stdout=subprocess.PIPE)
	if s.returncode:
		print('%s failed. retcode=%s' % (pdfPath, s.returncode))
		badFiles.append(pdfPath)

	if not doPoppler:
		continue
	s = subprocess.run([poplExe, '-l', numPages, pdfPath, textPath],  stdout=subprocess.PIPE)
	if s.returncode:
		print(s.returncode)

dt = time() - t0

print('=' * 80)
print('%d files %d bad %.1f sec' % (len(pathList), len(badFiles), dt))
for i, pdfPath in enumerate(badFiles):
	print('%4d: %s' % (i, pdfPath))
