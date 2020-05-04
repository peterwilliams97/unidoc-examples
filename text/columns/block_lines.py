import re
from sys import argv

# textBlock: lines built blk=3--------------------------
reBlk0 = re.compile(r'textBlock:\s+(.*)\s+blk=(\d+)')
txtBlk2 = '----------xxxx------------'

lineBlk0 = 'TextOutputDev.cc:1896 textBlock: lines built blk=1--------------------------'
assert reBlk0.search(lineBlk0)

# block 0: rot=0 {42.52 481.88 639.63 694.63} col=0 nCols=0 lines=3
reBlock = re.compile(r'block\s+(\d+):+\s+rot=(\d+)\s+\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}.*lines=(\d+)')
lineBlock = 'block 0: rot=0 {54.00 91.85 697.92 755.88} col=0 nCols=0 lines=1'
assert reBlock.search(lineBlock)

# line 0: base=120.24 {42.52 422.51 670.63 694.63} fontSize=24.00 "How people decide what they want to"
reLine = re.compile(r'line\s+(\d+)\s*:\s*base=(\S+)\s*\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}\s*fontSize=(\S+)\s*"(.*)"')
lineLine = '  line 0: base=120.24 {42.52 422.51 670.63 694.63} fontSize=24.00 "How people decide what they want to"'
assert reLine.search(lineLine)


def scanBlocks(path):
	n = 0
	blocks = []
	lines = []
	header = None
	state = 0
	with open(path, 'rt', errors='ignore') as f:
		for line in f:
			line = line[:-1]
			if not line:
				continue
			if state == 0:
				m = reBlk0.search(line)
				if m:
					state = 2
					header = m.group(1)
					lines = []
			elif state == 2:
				if txtBlk2 in line or reBlk0.search(line):
					state = 0
					assert lines
					blocks.append((header,lines))
				else:
					lines.append(line)
			# if state != 0:
			# 	print('state=%d: %s' % (state, line[:-1]))

	return blocks


def parseLine(line):
	m = reLine.search(line)
	assert m, '>>>%s<<<' % line
	llx = float(m.group(1))
	urx = float(m.group(2))
	lly = float(m.group(3))
	ury = float(m.group(4))
	base = float(m.group(5))
	text = m.group(6)
	return llx, urx, lly, ury, base, text, line

def parseBlock(line):
	m = reBlock.search(line)
	assert m, '>>>%s<<<' % line
	idx = int(m.group(1))
	rot = int(m.group(2))
	llx = float(m.group(3))
	urx = float(m.group(4))
	lly = float(m.group(5))
	ury = float(m.group(6))
	# base = float(m.group(7))
	nLines = int(m.group(7))
	return idx, rot, llx, urx, lly, ury, nLines

def parseBlockLines(lines):
	# print('parseBlockLines: lines=%d' % len(lines))
	assert lines
	# for ln in lines[1:]:
	# 	print(parseLine(ln))
	return parseBlock(lines[0]), [parseLine(ln) for ln in lines[1:]]


def scan(path):
	print('scan: %s -----------------' % path)
	blocks = scanBlocks(path)
	# print('scan: blocks=%d' % len(blocks))
	return [(header, lines, parseBlockLines(lines)) for header, lines in blocks]


blocks1 = scan(argv[1])
blocks2 = scan(argv[2])
print('%s %d blocks' % (argv[1], len(blocks1)))
print('%s %d blocks' % (argv[2], len(blocks2)))


def showLines(header, lines):
	line0 = '%d lines ' % len(lines)
	line0 += 'x' * (80 - len(line0))
	line1 = '+' * 80
	lines = [header, line0] + lines + [line1]
	return '\n'.join(lines)


TOL = 0.1
def equal(x1, x2):
	return abs(x1 - x2) < TOL

n = min(len(blocks1), len(blocks2))

for i in range(n):
	header1, lines1, (b1, blk1) = blocks1[i]
	header2, lines2, (b2, blk2) = blocks2[i]
	print('++ block %2d: %2d %2d entries ----------- "%s" "%s"' % (
		i,  len(blk1), len(blk2), header1, header2))

print('=' * 80)
for i in range(n):
	header1, lines1, (b1, blk1) = blocks1[i]
	header2, lines2, (b2, blk2) = blocks2[i]
	assert header1 == header2, (header1, header2)
	msg =  'i=%d "%s" blk1=%d blk2=%d\nlines1=\n%s\nlines2=\n%s' % (
		i, header1, len(blk1), len(blk2), showLines(header1, lines1), showLines(header2, lines2))
	assert len(blk1) == len(blk2), msg
	m = len(blk1)

	print('block %2d: %d entries ----------- "%s"' % (i,  m, header1))

	idx1, rot1, llx1, urx1, lly1, ury1, nLines1 = b1
	idx2, rot2, llx2, urx2, lly2, ury2, nLines2 = b2
	assert idx1 == idx2, msg
	assert llx1 == llx2, msg
	assert urx1 == urx2, msg
	assert nLines1 == nLines2, msg

	for j in range(m):
		# print('blk1=%d %s' % (len(blk1[j]), blk1[j]))
		llx1, urx1, lly1, ury1, base1, text1, line1 = blk1[j]
		llx2, urx2, lly2, ury2, base2, text2, line2 = blk2[j]
		msg = 'j=%d\n\tblk1=%s\n\tblk2=%s\nlines1=\n%s\nlines2=\n%s' % (j,
				blk1[j], blk2[j], showLines(header1, lines1), showLines(header2,lines2))
		# print('line %2d: %s' % (j, msg))
		assert equal(llx1, llx2), msg
		assert equal(urx1, urx2), msg
		# assert lly1 == lly2, msg
		# assert ury1 == ury2, msg
		assert equal(base1, base2), msg
		assert text1 == text2, msg

