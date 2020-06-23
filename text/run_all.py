import re
from sys import argv
from glob import glob
from os.path import basename, join, abspath, splitext, expanduser
from os import makedirs
import subprocess
from time import time

poplDir = 'popl'
poplExe = expanduser('~/pdf/poppler/build/utils/pdftotext')
poplExe = 'pdftotext'
uniExe = './pdf_extract_text'
makedirs(poplDir, exist_ok=True)

numPages = '10'
doPoppler = True


def testUni(pathList):
	print("UniDoc: %d files" % len(pathList))
	badFiles= []
	t0 = time()
	for i, pdfPath in enumerate(pathList):
		base = basename(pdfPath)
		base, _ = splitext(base)
		textPath = join(poplDir, '%s.txt' % base)
		print('%4d of %d: %s' % (i+1, len(pathList), pdfPath))
		s = subprocess.run([uniExe, '-l', numPages,  pdfPath], stdout=subprocess.PIPE)
		if s.returncode:
			print('%s failed. retcode=%s' % (pdfPath, s.returncode))
			badFiles.append(pdfPath)
	dt = time() - t0
	return dt, badFiles


def testPop(pathList):
	print("Poppler: %d files" % len(pathList))
	badFiles= []
	t0 = time()
	for i, pdfPath in enumerate(pathList):
		base = basename(pdfPath)
		base, _ = splitext(base)
		print('%4d of %d: %s' % (i+1, len(pathList), pdfPath))
		textPath = join(poplDir, '%s.txt' % base)
		s = subprocess.run([poplExe, '-l', numPages, pdfPath,
						textPath],  stdout=subprocess.PIPE)
		if s.returncode:
			print('%s failed. retcode=%s' % (pdfPath, s.returncode))
			badFiles.append(pdfPath)
	dt = time() - t0
	return dt, badFiles


def report(title, dt, badFiles):
	print("%s %s" % (title, ('-' * (80 - len(title)))))
	print('%d files %d bad %.1f sec' % (len(pathList), len(badFiles), dt))
	for i, pdfPath in enumerate(badFiles):
		print('%4d: %s' % (i, pdfPath))


pathList = []
for i, a in enumerate(argv[1:]):
	paths = glob(a)
	pathList.extend(paths)

print('=' * 80)
dtU, badFilesU = testUni(pathList)
if doPoppler:
	dtP, badFilesP = testPop(pathList)
report("UniDoc", dtU, badFilesU)
if doPoppler:
	report("Poppler", dtP, badFilesP)


