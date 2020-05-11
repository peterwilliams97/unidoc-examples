import re
from sys import argv
from glob import glob
from os.path import basename, join, abspath, splitext, expanduser
import subprocess

pathList = []
for i, a in enumerate(argv[1:]):
	paths = glob(a)
	pathList.extend(paths)
	# print('%3d: %3d %3d %s' % (i, len(paths), len(pathList), paths))

poplDir = 'popl'
poplExe = expanduser('~/pdf/poppler/build/utils/pdftotext')
# s = subprocess.check_output(["echo", "Hello World!"])
# print("s = " + s)

numPages = '10'
badFiles = []
for i, pdfPath in enumerate(pathList):
	base = basename(pdfPath)
	base, _ = splitext(base)
	textPath = join(poplDir, '%s.txt' % base)
	print('%4d of %d: %40s -- %s' % (i, len(pathList), pdfPath, textPath))
	s = subprocess.run(['./columns', '-l', numPages, pdfPath],  stdout=subprocess.PIPE)
	if s.returncode:
		print('%s failed. retcode=%s' % (pdfPath, s.returncode))
		badFiles.append(pdfPath)

	s = subprocess.run([poplExe, '-l', numPages, pdfPath,
                     textPath],  stdout=subprocess.PIPE)
	if s.returncode:
		print(s.returncode)

print('=' * 80)
print('%d files %d bad' % (len(pathList), len(badFiles)))
for i, pdfPath in enumerate(badFiles):
	print('%4d: %s' % (i, pdfPath))
