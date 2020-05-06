import re
from sys import argv

# [INFO]  text_block.go:276 textBlock: before discardDuplicatedText blk=0--------------------------
#  block 0: rot=0 {143.54 468.45 741.93 756.27} col=0 nCols=0 lines=0 pools=1 minBaseIdx=24 maxBaseIdx=24
#  pool 0: baseIdx=24 len=5
#   word 0: serial=0 base=99.96 {143.54 177.69 741.93 756.27} fontsize=14.35 "High"
#   word 1: serial=0 base=99.96 {183.07 271.98 741.93 756.27} fontsize=14.35 "Performance"
#   word 2: serial=0 base=99.96 {277.36 350.01 741.93 756.27} fontsize=14.35 "Document"
#   word 3: serial=0 base=99.96 {355.38 403.87 741.93 756.27} fontsize=14.35 "Layout"
#   word 4: serial=0 base=99.96 {409.23 468.45 741.93 756.27} fontsize=14.35 "Analysis"
# [INFO]  text_block.go:278 ----------xxxx------------

# textBlock: lines built blk=3--------------------------
reBlk0 = re.compile(r'textBlock:\s+(.*)\s+blk=(\d+)')
txtBlk2 = '----------xxxx------------'

lineBlk0 = 'TextOutputDev.cc:1896 textBlock: lines built blk=1--------------------------'
assert reBlk0.search(lineBlk0)

reBlock = re.compile(r'block\s+(\d+):+\s+rot=(\d+)\s+\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}.*lines=(-?\d+)\s+pools=(-?\d+)')
lineBlock = 'block 0: rot=0 {143.54 468.45 741.93 756.27} col=0 nCols=0 lines=0 pools=1 minBaseIdx=- maxBaseIdx=24'
assert reBlock.search(lineBlock)

rePool = re.compile(r'pool\s+(\d+)\s*:\s*baseIdx=(-?\d+)\s+len=(\d+)')
linePool = 'pool 0: baseIdx=24 len=5'
assert rePool.search(linePool)

reLine = re.compile(r'line\s+(\d+)\s*:\s*serial\s*=\s*\d+\s*base=\s*(\S+)\s*\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}\s*fontsize=(\S+)\s*"(.*)"')
lineLine = 'line 0: serial=0 base=99.96 {143.54 468.45 741.93 756.27} fontsize=14.35 "High Performance Document Layout Analysis" col = 0 []'
assert reLine.search(lineLine)

# word 0: serial=0 base=99.96 {143.54 177.69 741.93 756.27} fontsize=14.35 "High"
reWord = re.compile(r'word\s+(\d+)\s*:\s*serial\s*=\s*\d+\s+base=\s*(\S+)\s*\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}\s*fontsize\s*=\s*(\S+)\s*"(.*)"')
lineWord = 'word   0: serial=0 base=99.96 {143.54 177.69 741.93 756.27} fontsize=14.35 "High"'
assert reWord.search(lineWord)

def parseBlock(i, line):
	m = reBlock.search(line)
	assert m, '%d: >>>%s<<<' % (i, line)
	idx = int(m.group(1))
	rot = int(m.group(2))
	llx = float(m.group(3))
	urx = float(m.group(4))
	lly = float(m.group(5))
	ury = float(m.group(6))
	# base = float(m.group(7))
	nLines = int(m.group(7))
	nPools = int(m.group(8))
	return idx, rot, llx, urx, lly, ury, nLines, nPools

def parsePool(i, line):
	m = rePool.search(line)
	assert m, '%d: >>>%s<<<' % (i, line)
	idx = int(m.group(1))
	baseIdx = int(m.group(2))
	nWords = float(m.group(3))
	assert nWords > 0, (i, line)
	return idx, baseIdx, nWords, line

def parseLine(i, line):
	m = reLine.search(line)
	assert m, '%d: >>>%s<<<' % (i, line)
	base = float(m.group(1))
	llx = float(m.group(2))
	urx = float(m.group(3))
	lly = float(m.group(4))
	ury = float(m.group(5))
	fontsize = float(m.group(6))
	text = m.group(7)
	return base, llx, urx, lly, ury, fontsize, text, line

def parseWord(i, line):
	'word   0: serial=0 base=99.96 {143.54 177.69 741.93 756.27} fontsize=14.35 "High"'
	m = reWord.search(line)
	assert m, '%d: >>>%s<<<' % (i, line)
	try:
		idx = int(m.group(1))
		base = float(m.group(2))
		llx = float(m.group(3))
		urx = float(m.group(4))
		lly = float(m.group(5))
		ury = float(m.group(6))
		fontsize = float(m.group(7))
		text = m.group(8)
	except Exception as e:
		print(e, line, m.groups())
		raise
	return idx, base, llx, urx, lly, ury, fontsize, text

def scan(path):
	print('scan: %s -----------------' % path)
	n = 0
	blocks = []
	lines = []
	# words = []
	nLines = 0
	# nWords = 0
	header = None
	state = 0
	oldState = 0
	with open(path, 'rt', errors='ignore') as f:
		for i1, line in enumerate(f):
			i = i1 + 1
			line = line[:-1].strip()
			if not line:
				continue

			if state == 2 and len(lines) == nLines:
				blocks.append((header, block,  lines))
				state = 0
				lines = []
				nLines = 0

			if state == 0:
				m = reBlk0.search(line)
				if m:
					header = m.group(1)
					state = 1
			elif state == 1:
				block = parseBlock(i, line)
				state = 2
				nLines = block[6]
				lines = []
			elif state == 2:
				l1ne = parseLine(i, line)
				lines.append(l1ne)

			if state != 0:
				# print('state=%d->%d: %2d of %2d lines >>%s<<' % (
				# 	oldState, state, len(lines), nLines, line))
				assert len(lines) <= nLines
			oldState = state

	return blocks

def showLines(header, lines):
	line0 = '%d lines ' % len(lines)
	line0 += 'x' * (80 - len(line0))
	line1 = '+' * 80
	lines = [header, line0] + ['>>%s<<' % ln[-1] for ln in lines] + [line1]
	return '\n'.join(lines)

def showPools(header, pools):
	line0 = '%d pools ' % len(pools)
	line0 += 'x' * (80 - len(line0))
	line1 = '+' * 80
	# (idx1, baseIdx1, nWords1, line1), words1
	lines = [header, line0] + ['>>%s<<' % l[0][3] for l in pools] + [line1]
	# lines = [header, line0, line1]
	return '\n'.join(lines)

TOL = 0.1
def equal(x1, x2):
	return abs(x1 - x2) < TOL

blocks1 = scan(argv[1])
blocks2 = scan(argv[2])
print('%s %d blocks' % (argv[1], len(blocks1)))
print('%s %d blocks' % (argv[2], len(blocks2)))
# assert len(blocks1) == len(blocks2)
n = min(len(blocks1), len(blocks2))
for i in range(n):
	header1, _, lines1 = blocks1[i]
	header2, _, lines2 = blocks2[i]
	print('++ block %2d: %2d %2d entries ----------- "%s" "%s"' % (
		i,  len(lines1), len(lines2), header1, header2))
print('=' * 80)

for i in range(n):
	header1, blk1, lines1 = blocks1[i]
	header2, blk2, lines2 = blocks2[i]
	assert header1 == header2, (header1, header2)
	msg =  'i=%d "%s" blk1=%d blk2=%d\nlines1=\n%s\nliness2=\n%s' % (
		i, header1, len(lines1), len(lines2),
		showLines(header1, lines1),
		showLines(header2, lines2))

	print('block %2d ----------- "%s"\n\t%s\n\t%s' % (i, header1, list(blk1), list(blk2)))
	# print('blk1=%d %s' % (len(blk1), list(blk1)))
	assert len(lines1) == len(lines2), msg
	m = len(lines1)

	idx1, rot1, llx1, urx1, lly1, ury1, nLines1, nPools1 = blk1
	idx2, rot2, llx2, urx2, lly2, ury2, nLines2, nPools2 = blk2
	assert idx1 == idx2, msg
	assert llx1 == llx2, msg
	assert urx1 == urx2, msg
	assert nLines1 == nLines2, msg
	assert nLines1 == len(lines1), msg
	assert nLines2 == len(lines2), msg

	for j in range(m):
		# print('blk1=%d %s' % (len(blk1[j]), blk1[j]))
		base1, lx1, urx1, lly1, ury1, fontsize1, text1, line1 = lines1[j]
		base2, lx2, urx2, lly2, ury2, fontsize2, text2, line2 = lines2[j]
		msg = 'j=%d\n\tblk1=%s\n\tblk2=%s' % (j, lines1[j], lines2[j])

		assert base1 == base2, msg
		assert llx1 == llx2, msg
		assert urx1 == urx2, msg
		assert fontsize1 == fontsize1, msg
		assert text1 == text2, msg

print('/' * 80)
