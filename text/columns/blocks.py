import re
from sys import argv

# textBlock: lines built blk=3--------------------------
reBlk0 = re.compile(r'textBlock:\s+lines\s+built\s+blk=(\d+)')
txtBlk1 = '------------------------'
txtBlk2 = '----------xxxx------------'

lineBlk0 = 'TextOutputDev.cc:1896 textBlock: lines built blk=1--------------------------'
assert reBlk0.search(lineBlk0)


# 0: {404.76 483.00 655.20 664.20} base=136.80 "Kagan, Elena (1960-)"
reLine = re.compile(r'\{\s*(\S+)\s+(\S+)\s+(\S+)\s*(\S+)\s*\}\s*base=(\S+)\s*"(.*)"')

lineLine = '0: {404.76 483.00 655.20 664.20} base=136.80 "Kagan, Elena (1960-)"'
assert reLine.search(lineLine)


def scanBlocks(path):
	n = 0
	blocks = []
	lines = []
	header = None
	state = 0
	with open(path, 'rt') as f:
		for line in f:
			line = line[:-1]
			if not line:
				continue
			if state == 0:
				m = reBlk0.search(line)
				if m:
					state = 1
					header = line
			elif state == 1:
				if txtBlk1 in line:
					state = 2
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

def parseBlock(lines):
	# print('parseBlock: lines=%d' % len(lines))
	assert lines
	# for ln in lines[1:]:
	# 	print(parseLine(ln))
	return [parseLine(ln) for ln in lines[1:]]


def scan(path):
	print('scan: %s -----------------' % path)
	blocks = scanBlocks(path)
	# print('scan: blocks=%d' % len(blocks))
	return [(header, lines, parseBlock(lines)) for header, lines in blocks]

	
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

n = min(len(blocks1), len(blocks2))

for i in range(n):
	header1, lines1, blk1 = blocks1[i]
	header2, lines2, blk2 = blocks2[i]
	assert len(blk1) == len(blk2), 'i=%d blk1=%d blk2=%d\nlines1=\n%s\nlines2=\n%s' % (
		i, len(blk1), len(blk2), showLines(header1, lines1), showLines(header2, lines2))
	m = len(blk1)

	print('block %2d: %d entries -----------' % (i, m))
	
	for j in range(m):
		# print('blk1=%d %s' % (len(blk1[j]), blk1[j]))
		llx1, urx1, lly1, ury1, base1, text1, line1 = blk1[j] 
		llx2, urx2, lly2, ury2, base2, text2, line2 = blk2[j] 
		msg = 'j=%d\n\tblk1=%s\n\tblk2=%s\nlines1=\n%s\nlines2=\n%s' % (j, 
				blk1[j], blk2[j], showLines(header1, lines1), showLines(header2,lines2))
		# print('line %2d: %s' % (j, msg))
		assert llx1 == llx2, msg
		assert urx1 == urx2, msg
		# assert lly1 == lly2, msg
		# assert ury1 == ury2, msg
		assert base1 == base2, msg
		assert text1 == text2, msg

